package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Mekski/reqradar/internal/fit"
	"github.com/Mekski/reqradar/internal/sentiment"
	"github.com/Mekski/reqradar/internal/store"
)

// Request body caps for the handlers that read user-supplied text into memory
// (and forward it to Gemini). Generous vs. real inputs, but bounded so a giant
// body can't exhaust memory once the API is public.
const (
	maxResumeBody = 4 << 20 // 4 MiB — extracted resume text (real resumes are KBs)
	maxJSONBody   = 1 << 20 // 1 MiB — fit-score JSON, incl. a pasted JD
)

// llmCallTimeout bounds a detached LLM call. It sits above the Gemini HTTP
// client's own 60s timeout so the model call + the cache write can finish even
// when the browser tab that triggered them goes away.
const llmCallTimeout = 75 * time.Second

// detachedLLMCtx derives a context from the request that survives client
// disconnect (context.WithoutCancel) but is still bounded by llmCallTimeout. This
// keeps the "one model call per unique input, ever" invariant intact: if the user
// closes the tab mid-score, the result is still generated and cached rather than
// thrown away and re-paid for on the next attempt.
func detachedLLMCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), llmCallTimeout)
}

// Server is the dashboard REST API. v1 is single-user, so it operates against
// one fixed userID. Auth is defense-in-depth: an optional static bearer token
// (REQRADAR_API_TOKEN) gates /api/* when set, and in prod Caddy basic-auth + a
// localhost bind front it (DESIGN §3.3). Multi-user later means deriving userID
// from a session.
type Server struct {
	store     *store.Store
	log       *slog.Logger
	userID    int64
	fit       *fit.Service
	sentiment *sentiment.Service
}

// ServerConfig carries the cross-cutting HTTP concerns (auth + CORS) so they
// aren't hard-coded in the handler.
type ServerConfig struct {
	APIToken   string // when non-empty, /api/* requires "Authorization: Bearer <token>"
	CORSOrigin string // Access-Control-Allow-Origin; "*" for local dev
}

func NewServer(st *store.Store, log *slog.Logger, userID int64, fitSvc *fit.Service, sentSvc *sentiment.Service, cfg ServerConfig) http.Handler {
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
	mux.HandleFunc("GET /api/companies/{id}/seasonality", s.seasonality)
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
	return cors(cfg.CORSOrigin, requireToken(cfg.APIToken, mux))
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

// cors allows the dashboard origin to call the API (default "*" for local dev;
// set REQRADAR_CORS_ORIGIN to the dashboard's origin in prod). It is the outermost
// wrapper so an OPTIONS preflight (which carries no Authorization header) short-
// circuits before the token check.
func cors(origin string, next http.Handler) http.Handler {
	if origin == "" {
		origin = "*"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireToken gates /api/* behind a static bearer token when one is configured.
// An empty token means open (local dev) — the dashboard then talks to the API
// directly. /healthz is always public so health checks need no secret.
func requireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("Authorization") != want {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
