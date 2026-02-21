package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
)

// URLStore abstracts persistence so handlers can be tested with a fake store.
type URLStore interface {
	SaveURL(code, longURL string) error
	GetURL(code string) (string, error)
}

type Server struct {
	store URLStore
}

// dbStore implements URLStore using MySQL.
type dbStore struct{ db *sql.DB }

func (d *dbStore) SaveURL(code, longURL string) error { return saveURL(d.db, code, longURL) }
func (d *dbStore) GetURL(code string) (string, error) { return getURL(d.db, code) }

type ShortenRequest struct {
	URL string `json:"url"`
}

type ShortenResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

func NewServer(store URLStore) *Server {
	return &Server{store: store}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}

func (s *Server) shorten(w http.ResponseWriter, r *http.Request) {
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

	for i := 0; i < 5; i++ { // retry a few times in case of collision or transient failure
		c, err := generateCode(6)
		if err != nil {
			http.Error(w, "failed to generate code", http.StatusInternalServerError)
			return
		}

		err = s.store.SaveURL(c, req.URL)
		if err == nil {
			code = c
			break
		}

		// IMPORTANT: don't silently swallow DB errors — log them so we can debug.
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

	resp := ShortenResponse{
		Code:     code,
		ShortURL: "http://localhost:8080/" + code,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	// path like "/abc123" → code "abc123"
	code := r.URL.Path[1:]
	if code == "" || code == "health" || code == "shorten" {
		http.NotFound(w, r)
		return
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

	http.Redirect(w, r, longURL, http.StatusFound)
}

func generateCode(n int) (string, error) {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	out := make([]byte, n)

	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		out[i] = alphabet[num.Int64()]
	}
	return string(out), nil
}

func ensureSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS urls (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  code VARCHAR(16) NOT NULL UNIQUE,
  long_url TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`)
	return err
}

func saveURL(db *sql.DB, code, longURL string) error {
	_, err := db.Exec(`INSERT INTO urls (code, long_url) VALUES (?, ?)`, code, longURL)
	return err
}

func getURL(db *sql.DB, code string) (string, error) {
	var longURL string
	err := db.QueryRow(`SELECT long_url FROM urls WHERE code = ?`, code).Scan(&longURL)
	return longURL, err
}

func main() {
	dsn := "root:password@tcp(127.0.0.1:3306)/shortener?parseTime=true"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	if err := ensureSchema(db); err != nil {
		log.Fatal(err)
	}

	s := NewServer(&dbStore{db: db})

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/shorten", s.shorten)
	mux.HandleFunc("/", s.resolve) // catch-all for /{code}

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}