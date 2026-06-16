package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Mekski/reqradar/internal/llm"
)

// uploadResume stores resume text. The PDF is parsed in the browser (pdf.js, which
// handles LaTeX/Overleaf spacing far better than a Go extractor), so this just
// takes the extracted text.
func (s *Server) uploadResume(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxResumeBody)
	var in struct {
		Filename string `json:"filename"`
		Text     string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if len(strings.TrimSpace(in.Text)) < 50 {
		http.Error(w, "resume text too short — is this a scanned/image PDF?", http.StatusUnprocessableEntity)
		return
	}
	if in.Filename == "" {
		in.Filename = "resume.pdf"
	}
	resume, err := s.store.SaveResume(r.Context(), s.userID, in.Filename, in.Text)
	s.respond(w, resume, err)
}

func (s *Server) listResumes(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ListResumes(r.Context(), s.userID)
	s.respond(w, data, err)
}

func (s *Server) deleteResume(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteResume(r.Context(), s.userID, id); err != nil {
		s.respond(w, nil, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) fitJDs(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ScoreableJDs(r.Context(), s.userID)
	s.respond(w, data, err)
}

// fitStatus tells the UI whether scoring is available (an API key is set), so it
// can show a "configure GEMINI_API_KEY" hint instead of failing on submit.
func (s *Server) fitStatus(w http.ResponseWriter, _ *http.Request) {
	s.respond(w, map[string]bool{"configured": s.fit.Configured()}, nil)
}

// scoreFit scores a resume against a JD (a pasted jd_text, or a watchlist
// posting_id whose stored JD we use). Returns the cached or fresh result.
func (s *Server) scoreFit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	var in struct {
		ResumeID  int64  `json:"resume_id"`
		PostingID *int64 `json:"posting_id"`
		JDText    string `json:"jd_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ResumeID == 0 {
		http.Error(w, "resume_id is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	resumeText, err := s.store.ResumeText(ctx, s.userID, in.ResumeID)
	if err != nil {
		http.Error(w, "resume not found", http.StatusNotFound)
		return
	}

	jd := in.JDText
	if in.PostingID != nil {
		jd, err = s.store.PostingJD(ctx, *in.PostingID)
		if err != nil {
			s.respond(w, nil, err)
			return
		}
	}
	if jd == "" {
		http.Error(w, "no job description — paste one or pick a posting with stored JD text", http.StatusBadRequest)
		return
	}

	// Detach from the request so closing the tab mid-score still completes + caches
	// the Gemini call (otherwise the next attempt re-pays for the same pair).
	genCtx, cancel := detachedLLMCtx(ctx)
	defer cancel()
	result, cached, err := s.fit.Score(genCtx, resumeText, jd, in.PostingID)
	if errors.Is(err, llm.ErrNotConfigured) {
		http.Error(w, "fit scoring isn't configured yet — add GEMINI_API_KEY to .env and restart the api", http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		s.log.Error("fit score", "err", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if cached {
		w.Header().Set("X-Fit-Cache", "hit")
	}
	_, _ = w.Write(result)
}
