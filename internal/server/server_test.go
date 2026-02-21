package server

import (
	"bytes"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"url-shortener/internal/cache"
	"url-shortener/internal/store"
)

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

type failingStore struct{ err error }

func (f *failingStore) SaveURL(_, _ string) error              { return f.err }
func (f *failingStore) GetURL(_ string) (string, error) { return "", f.err }

func TestHealth(t *testing.T) {
	s := NewServer(newFakeStore(), cache.New(nil))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.Health(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("health: got status %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "ok\n" {
		t.Errorf("health: got body %q, want \"ok\\n\"", body)
	}
}

func TestShorten_Success(t *testing.T) {
	st := newFakeStore()
	s := NewServer(st, cache.New(nil))
	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Shorten(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("shorten: got status %d, want 200; body: %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("shorten: Content-Type want application/json, got %s", rec.Header().Get("Content-Type"))
	}
	resp := rec.Body.Bytes()
	if !bytes.Contains(resp, []byte("short_url")) || !bytes.Contains(resp, []byte("code")) {
		t.Errorf("shorten: response should contain short_url and code: %s", resp)
	}
	st.SaveURL("abc123", "https://stored.com")
	req2 := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	rec2 := httptest.NewRecorder()
	s.Resolve(rec2, req2)
	if rec2.Code != http.StatusFound {
		t.Errorf("resolve: got %d, want 302", rec2.Code)
	}
	if loc := rec2.Header().Get("Location"); loc != "https://stored.com" {
		t.Errorf("resolve: Location want https://stored.com, got %s", loc)
	}
}

func TestShorten_MethodNotAllowed(t *testing.T) {
	s := NewServer(newFakeStore(), cache.New(nil))
	req := httptest.NewRequest(http.MethodGet, "/shorten", nil)
	rec := httptest.NewRecorder()
	s.Shorten(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("shorten GET: got status %d, want 405", rec.Code)
	}
}

func TestShorten_InvalidJSON(t *testing.T) {
	s := NewServer(newFakeStore(), cache.New(nil))
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Shorten(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("shorten bad JSON: got status %d, want 400", rec.Code)
	}
}

func TestShorten_EmptyURL(t *testing.T) {
	s := NewServer(newFakeStore(), cache.New(nil))
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(`{"url":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Shorten(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("shorten empty url: got status %d, want 400", rec.Code)
	}
}

func TestShorten_StoreAlwaysFails(t *testing.T) {
	s := NewServer(&failingStore{err: errors.New("db unavailable")}, cache.New(nil))
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(`{"url":"https://example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Shorten(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("shorten when store fails: got status %d, want 500", rec.Code)
	}
	if body := rec.Body.String(); body != "" && !bytes.Contains(rec.Body.Bytes(), []byte("failed to create short url")) {
		t.Errorf("shorten 500 should mention failure: %s", body)
	}
}

func TestResolve_StoreError(t *testing.T) {
	s := NewServer(&failingStore{err: errors.New("connection refused")}, cache.New(nil))
	req := httptest.NewRequest(http.MethodGet, "/somecode", nil)
	rec := httptest.NewRecorder()
	s.Resolve(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("resolve when store errors: got status %d, want 500", rec.Code)
	}
}

func TestResolve_NotFound(t *testing.T) {
	s := NewServer(newFakeStore(), cache.New(nil))
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	s.Resolve(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("resolve missing code: got status %d, want 404", rec.Code)
	}
}

func TestResolve_ReservedPaths(t *testing.T) {
	s := NewServer(newFakeStore(), cache.New(nil))
	for _, path := range []string{"/", "/health", "/shorten"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.Resolve(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("resolve %q: got status %d, want 404", path, rec.Code)
		}
	}
}

// Ensure fakeStore and failingStore implement store.URLStore.
var _ store.URLStore = (*fakeStore)(nil)
var _ store.URLStore = (*failingStore)(nil)
