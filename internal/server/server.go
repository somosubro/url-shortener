package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

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

type LookupItem struct {
	Code      string    `json:"code"`
	ShortURL  string    `json:"short_url"`
	CreatedAt time.Time `json:"created_at"`
}

type LookupResponse struct {
	URL   string       `json:"url"`
	Items []LookupItem `json:"items"`
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

	if s.cache != nil {
		s.cache.Set(code, req.URL)
	}

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
	if code == "" || code == "health" || code == "shorten" || code == "lookup" {
		http.NotFound(w, r)
		return
	}

	log.Println("resolve:", code)

	if s.cache != nil {
		if longURL, ok := s.cache.Get(code); ok {
			log.Println("cache HIT:", code)
			http.Redirect(w, r, longURL, http.StatusFound)
			return
		}
		log.Println("cache MISS:", code)
	}

	longURL, err := s.store.GetURL(code)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if s.cache != nil {
		s.cache.Set(code, longURL)
		log.Println("cache SET:", code)
	}
	http.Redirect(w, r, longURL, http.StatusFound)
}

// Lookup handles GET /lookup?url=...
// Returns the most recent short codes created for that long URL.
func (s *Server) Lookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	longURL := r.URL.Query().Get("url")
	if longURL == "" {
		http.Error(w, "url query param is required", http.StatusBadRequest)
		return
	}

	results, err := s.store.LookupCodes(longURL, 10)
	if err != nil {
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]LookupItem, 0, len(results))
	for _, r := range results {
		items = append(items, LookupItem{
			Code:      r.Code,
			ShortURL:  "http://localhost:8080/" + r.Code,
			CreatedAt: r.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(LookupResponse{
		URL:   longURL,
		Items: items,
	})
}
