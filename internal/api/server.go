package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Mekski/reqradar/internal/fit"
	"github.com/Mekski/reqradar/internal/sentiment"
	"github.com/Mekski/reqradar/internal/store"
)

// Server is the dashboard REST API. v1 is single-user, so it operates against
// one fixed userID and has no auth of its own (Caddy basic-auth fronts it in
// prod; see DESIGN §3.3). Multi-user later means deriving userID from a session.
type Server struct {
	store     *store.Store
	log       *slog.Logger
	userID    int64
	fit       *fit.Service
	sentiment *sentiment.Service
}

func NewServer(st *store.Store, log *slog.Logger, userID int64, fitSvc *fit.Service, sentSvc *sentiment.Service) http.Handler {
	s := &Server{store: st, log: log, userID: userID, fit: fitSvc, sentiment: sentSvc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /api/companies", s.companies)
	mux.HandleFunc("POST /api/companies", s.addCompany)
	mux.HandleFunc("PATCH /api/companies/{id}", s.updateCompany)
	mux.HandleFunc("DELETE /api/companies/{id}", s.removeCompany)
	mux.HandleFunc("GET /api/companies/{id}/timeline", s.timeline)
	mux.HandleFunc("GET /api/companies/{id}/timing", s.timing)
	mux.HandleFunc("GET /api/companies/{id}/seasonality", s.seasonality)
	mux.HandleFunc("GET /api/postings", s.postings)
	mux.HandleFunc("GET /api/firehose", s.firehose)
	// Fit score (LLM): resume upload/list + JD picker + scoring.
	mux.HandleFunc("POST /api/resumes", s.uploadResume)
	mux.HandleFunc("GET /api/resumes", s.listResumes)
	mux.HandleFunc("DELETE /api/resumes/{id}", s.deleteResume)
	mux.HandleFunc("GET /api/fit/jds", s.fitJDs)
	mux.HandleFunc("GET /api/fit/status", s.fitStatus)
	mux.HandleFunc("POST /api/fit", s.scoreFit)
	// Sentiment (grounded LLM): the stored report + on-demand (re)generation.
	mux.HandleFunc("GET /api/companies/{id}/sentiment", s.getSentiment)
	mux.HandleFunc("POST /api/companies/{id}/sentiment", s.generateSentiment)
	return cors(mux)
}

func (s *Server) companies(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.WatchlistCompanies(r.Context(), s.userID)
	s.respond(w, data, err)
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	data, err := s.store.CompanyTimeline(r.Context(), id, 50)
	s.respond(w, data, err)
}

func (s *Server) timing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	data, err := s.store.CompanyTiming(r.Context(), id)
	s.respond(w, data, err)
}

// categoryGroups maps a filter key to the messy aggregator category labels.
var categoryGroups = map[string][]string{
	"swe": {"Software", "Software Engineering"},
	"ml":  {"AI/ML/Data", "Data Science, AI & Machine Learning"},
	"all": nil, // no filter
}

func (s *Server) seasonality(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	cat := r.URL.Query().Get("category")
	if cat == "" {
		cat = "swe" // default to SWE-intern roles
	}
	cats, known := categoryGroups[cat]
	if !known {
		cats = categoryGroups["swe"]
	}
	data, err := s.store.CompanySeasonality(r.Context(), id, cats)
	s.respond(w, data, err)
}

func (s *Server) postings(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.OpenPostings(r.Context(), s.userID, 200)
	s.respond(w, data, err)
}

func (s *Server) firehose(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.RecentFirehose(r.Context(), 200)
	s.respond(w, data, err)
}

func (s *Server) addCompany(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string   `json:"name"`
		Domain   string   `json:"domain"`
		Priority string   `json:"priority"`
		Aliases  []string `json:"aliases"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	defer tx.Rollback(ctx)
	id, err := s.store.UpsertCompany(ctx, tx, s.userID, store.CompanyInput{
		Name: in.Name, Domain: in.Domain, Priority: in.Priority, Source: "manual", Aliases: in.Aliases,
	})
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.respond(w, nil, err)
		return
	}
	s.respond(w, map[string]int64{"id": id}, nil)
}

func (s *Server) updateCompany(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Priority string `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Priority == "" {
		http.Error(w, "priority is required", http.StatusBadRequest)
		return
	}
	if err := s.store.UpdateCompanyTier(r.Context(), id, in.Priority); err != nil {
		s.respond(w, nil, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeCompany(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.RemoveWatchlistCompany(r.Context(), s.userID, id); err != nil {
		s.respond(w, nil, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// respond writes data as JSON, or a 500 on error. A nil slice serializes as [].
func (s *Server) respond(w http.ResponseWriter, data any, err error) {
	if err != nil {
		s.log.Error("handler", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// cors allows the Next.js dev server (localhost:3000) to call the API.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
