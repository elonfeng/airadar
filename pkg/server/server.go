package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/elonfeng/airadar/internal/store"
	"github.com/elonfeng/airadar/pkg/source"
	"github.com/elonfeng/airadar/pkg/trend"
)

// Server provides the HTTP API.
type Server struct {
	store   store.Store
	engine  *trend.Engine
	sources []source.Source
	port    int
}

// New creates a new HTTP server.
func New(s store.Store, engine *trend.Engine, sources []source.Source, port int) *Server {
	if port == 0 {
		port = 8080
	}
	return &Server{
		store:   s,
		engine:  engine,
		sources: sources,
		port:    port,
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/trends", s.handleTrends)
	mux.HandleFunc("/api/v1/items", s.handleItems)
	mux.HandleFunc("/api/v1/sources", s.handleSources)
	mux.HandleFunc("/api/v1/collect", s.handleCollect)

	addr := fmt.Sprintf(":%d", s.port)
	fmt.Printf("airadar server listening on %s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleTrends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	trends, err := s.store.ListTrends(r.Context(), store.TrendListOpts{
		MinScore: 0,
		Limit:    50,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  trends,
		"count": len(trends),
	})
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	opts := store.ListOpts{Limit: 100}
	if src := r.URL.Query().Get("source"); src != "" {
		opts.Source = source.SourceType(src)
	}
	if since := r.URL.Query().Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			opts.Since = t
		}
	}

	items, err := s.store.ListItems(r.Context(), opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  items,
		"count": len(items),
	})
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	counts, err := s.store.CountItemsBySource(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type sourceInfo struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Items   int    `json:"items"`
	}

	var infos []sourceInfo
	for _, src := range s.sources {
		infos = append(infos, sourceInfo{
			Name:    string(src.Name()),
			Enabled: true,
			Items:   counts[src.Name()],
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  infos,
		"count": len(infos),
	})
}

func (s *Server) handleCollect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	ctx := r.Context()
	results := make(map[string]int)
	var errs []string

	for _, src := range s.sources {
		items, err := src.Collect(ctx)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", src.Name(), err))
			continue
		}
		if err := s.store.UpsertItems(ctx, items); err != nil {
			errs = append(errs, fmt.Sprintf("%s store: %v", src.Name(), err))
			continue
		}
		results[string(src.Name())] = len(items)
	}

	resp := map[string]any{"collected": results}
	if len(errs) > 0 {
		resp["errors"] = errs
	}

	writeJSON(w, http.StatusOK, resp)
}

// RunTrendDetection triggers trend detection. Used by the scheduler.
func (s *Server) RunTrendDetection(ctx context.Context) ([]store.Trend, error) {
	return s.engine.Detect(ctx)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
