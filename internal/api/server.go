package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Mekski/reqradar/internal/store"
)

// Server is the dashboard REST API. v1 is single-user, so it operates against
// one fixed userID and has no auth of its own (Caddy basic-auth fronts it in
// prod; see DESIGN §3.3). Multi-user later means deriving userID from a session.
type Server struct {
	store  *store.Store
	log    *slog.Logger
	userID int64
}

func NewServer(st *store.Store, log *slog.Logger, userID int64) http.Handler {
	s := &Server{store: st, log: log, userID: userID}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /api/companies", s.companies)
	mux.HandleFunc("GET /api/companies/{id}/timeline", s.timeline)
	mux.HandleFunc("GET /api/companies/{id}/timing", s.timing)
	mux.HandleFunc("GET /api/postings", s.postings)
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

func (s *Server) postings(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.OpenPostings(r.Context(), s.userID, 200)
	s.respond(w, data, err)
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
