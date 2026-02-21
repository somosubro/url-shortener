package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"url-shortener/internal/cache"
	"url-shortener/internal/codegen"
	"url-shortener/internal/store"
)

// Server holds HTTP handlers and dependencies (store + cache).
type Server struct {
	store store.URLStore
	cache *cache.Cache
}

// ShortenRequest is the JSON body for POST /shorten.
type ShortenRequest struct {
	URL string `json:"url"`
}

// ShortenResponse is the JSON response for POST /shorten.
type ShortenResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

const (
	shortCodeLen = 6
	shortenRetry = 5
)

// NewServer returns a Server that uses the given store and cache.
func NewServer(store store.URLStore, cache *cache.Cache) *Server {
	return &Server{store: store, cache: cache}
}

// Health handles GET /health.
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}

// Shorten handles POST /shorten.
func (s *Server) Shorten(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var req ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	var (
		code    string
		lastErr error
	)

	for i := 0; i < shortenRetry; i++ {
		c, err := codegen.GenerateCode(shortCodeLen)
		if err != nil {
			http.Error(w, "failed to generate code", http.StatusInternalServerError)
			return
		}

		err = s.store.SaveURL(c, req.URL)
		if err == nil {
			code = c
			break
		}

		lastErr = err
		log.Println("saveURL failed:", err)
	}

	if code == "" {
		msg := "failed to create short url"
		if lastErr != nil {
			msg = msg + ": " + lastErr.Error()
		}
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	s.cache.Set(code, req.URL)

	resp := ShortenResponse{
		Code:     code,
		ShortURL: "http://localhost:8080/" + code,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Resolve handles GET /{code} and redirects to the long URL.
func (s *Server) Resolve(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Path[1:]
	if code == "" || code == "health" || code == "shorten" {
		http.NotFound(w, r)
		return
	}

	log.Println("resolve:", code)

	if longURL, ok := s.cache.Get(code); ok {
		log.Println("cache HIT:", code)
		http.Redirect(w, r, longURL, http.StatusFound)
		return
	}
	log.Println("cache MISS:", code)

	longURL, err := s.store.GetURL(code)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.cache.Set(code, longURL)
	log.Println("cache SET:", code)
	http.Redirect(w, r, longURL, http.StatusFound)
}
