package store

import "database/sql"

// URLStore abstracts persistence for short code → URL mapping.
// Implementations can use MySQL, memory, etc., for testability.
type URLStore interface {
	SaveURL(code, longURL string) error
	GetURL(code string) (string, error)
}

// DBStore implements URLStore using MySQL.
type DBStore struct{ db *sql.DB }

// NewDBStore returns a MySQL-backed URLStore.
func NewDBStore(db *sql.DB) *DBStore {
	return &DBStore{db: db}
}

func (d *DBStore) SaveURL(code, longURL string) error {
	return saveURL(d.db, code, longURL)
}

func (d *DBStore) GetURL(code string) (string, error) {
	return getURL(d.db, code)
}

// EnsureSchema creates the urls table if it does not exist.
func EnsureSchema(db *sql.DB) error {
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
