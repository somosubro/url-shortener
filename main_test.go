package main

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeStore is an in-memory URLStore for testing.
type fakeStore struct {
	mu   sync.RWMutex
	urls map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{urls: make(map[string]string)}
}

func (f *fakeStore) SaveURL(code, longURL string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls[code] = longURL
	return nil
}

func (f *fakeStore) GetURL(code string) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	url, ok := f.urls[code]
	if !ok {
		return "", sql.ErrNoRows
	}
	return url, nil
}

func TestGenerateCode(t *testing.T) {
	for _, n := range []int{1, 6, 12} {
		code, err := generateCode(n)
		if err != nil {
			t.Fatalf("generateCode(%d): %v", n, err)
		}
		if len(code) != n {
			t.Errorf("generateCode(%d): got len %d, want %d", n, len(code), n)
		}
		const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
		for _, c := range code {
			if !contains(alphabet, c) {
				t.Errorf("generateCode: invalid char %q in %q", c, code)
			}
		}
	}
}

func contains(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

func TestHealth(t *testing.T) {
	s := NewServer(newFakeStore())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.health(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("health: got status %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "ok\n" {
		t.Errorf("health: got body %q, want \"ok\\n\"", body)
	}
}

func TestShorten_Success(t *testing.T) {
	store := newFakeStore()
	s := NewServer(store)
	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.shorten(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("shorten: got status %d, want 200; body: %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("shorten: Content-Type want application/json, got %s", rec.Header().Get("Content-Type"))
	}
	// Response should contain short_url and code; we can't assert exact code (random).
	resp := rec.Body.Bytes()
	if !bytes.Contains(resp, []byte("short_url")) || !bytes.Contains(resp, []byte("code")) {
		t.Errorf("shorten: response should contain short_url and code: %s", resp)
	}
	// Resolve the returned code to verify it was stored (we'd need to parse JSON; simpler: call resolve with a known code we inject)
	store.SaveURL("abc123", "https://stored.com")
	req2 := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	rec2 := httptest.NewRecorder()
	s.resolve(rec2, req2)
	if rec2.Code != http.StatusFound {
		t.Errorf("resolve: got %d, want 302", rec2.Code)
	}
	if loc := rec2.Header().Get("Location"); loc != "https://stored.com" {
		t.Errorf("resolve: Location want https://stored.com, got %s", loc)
	}
}

func TestShorten_MethodNotAllowed(t *testing.T) {
	s := NewServer(newFakeStore())
	req := httptest.NewRequest(http.MethodGet, "/shorten", nil)
	rec := httptest.NewRecorder()
	s.shorten(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("shorten GET: got status %d, want 405", rec.Code)
	}
}

func TestShorten_InvalidJSON(t *testing.T) {
	s := NewServer(newFakeStore())
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.shorten(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("shorten bad JSON: got status %d, want 400", rec.Code)
	}
}

func TestShorten_EmptyURL(t *testing.T) {
	s := NewServer(newFakeStore())
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(`{"url":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.shorten(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("shorten empty url: got status %d, want 400", rec.Code)
	}
}

func TestResolve_NotFound(t *testing.T) {
	s := NewServer(newFakeStore())
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	s.resolve(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("resolve missing code: got status %d, want 404", rec.Code)
	}
}

func TestResolve_ReservedPaths(t *testing.T) {
	s := NewServer(newFakeStore())
	for _, path := range []string{"/", "/health", "/shorten"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.resolve(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("resolve %q: got status %d, want 404", path, rec.Code)
		}
	}
}
