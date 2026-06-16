package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Mekski/reqradar/internal/fit"
	"github.com/Mekski/reqradar/internal/llm"
)

const maxResumeBytes = 10 << 20 // 10 MB

// uploadResume accepts a multipart PDF (field "resume"), extracts its text, and
// stores it for the user.
func (s *Server) uploadResume(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxResumeBytes); err != nil {
		http.Error(w, "invalid upload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("resume")
	if err != nil {
		http.Error(w, "missing 'resume' file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxResumeBytes))
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	text, err := fit.ExtractText(data)
	if err != nil {
		// Extraction failures are user-fixable (e.g. a scanned PDF) — 422, not 500.
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	resume, err := s.store.SaveResume(r.Context(), s.userID, header.Filename, text)
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

	result, cached, err := s.fit.Score(ctx, resumeText, jd, in.PostingID)
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
