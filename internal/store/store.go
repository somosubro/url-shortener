package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

// URLStore abstracts persistence for short code → URL mapping.
type URLStore interface {
	SaveURL(code, longURL string) error
	GetURL(code string) (string, error)

	// LookupCodes returns recent short codes created for a given long URL.
	LookupCodes(longURL string, limit int) ([]LookupResult, error)
}

// LookupResult is one row returned by LookupCodes.
type LookupResult struct {
	Code      string
	CreatedAt time.Time
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

func (d *DBStore) LookupCodes(longURL string, limit int) ([]LookupResult, error) {
	if limit <= 0 {
		limit = 10
	}
	return lookupCodes(d.db, longURL, limit)
}

// EnsureSchema creates the urls table if it does not exist,
// and best-effort upgrades it to include long_url_hash + index.
func EnsureSchema(db *sql.DB) error {
	// Create (new installs get the full schema)
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS urls (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  code VARCHAR(16) NOT NULL UNIQUE,
  long_url TEXT NOT NULL,
  long_url_hash CHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_long_url_hash (long_url_hash)
);`)
	if err != nil {
		return err
	}

	// Best-effort upgrade for older tables that lack the new column/index.
	// Ignore "duplicate column" / "duplicate key name" errors.
	if _, err := db.Exec(`ALTER TABLE urls ADD COLUMN long_url_hash CHAR(64) NOT NULL DEFAULT ''`); err != nil {
		if !isIgnorableSchemaErr(err) {
			return err
		}
	}
	if _, err := db.Exec(`CREATE INDEX idx_long_url_hash ON urls (long_url_hash)`); err != nil {
		if !isIgnorableSchemaErr(err) {
			return err
		}
	}

	return nil
}

func isIgnorableSchemaErr(err error) bool {
	me, ok := err.(*mysqlDriver.MySQLError)
	if !ok {
		return false
	}
	// 1060 = Duplicate column name
	// 1061 = Duplicate key name
	return me.Number == 1060 || me.Number == 1061
}

func urlHash(u string) string {
	sum := sha256.Sum256([]byte(u))
	return hex.EncodeToString(sum[:])
}

func saveURL(db *sql.DB, code, longURL string) error {
	h := urlHash(longURL)
	_, err := db.Exec(`INSERT INTO urls (code, long_url, long_url_hash) VALUES (?, ?, ?)`, code, longURL, h)
	return err
}

func getURL(db *sql.DB, code string) (string, error) {
	var longURL string
	err := db.QueryRow(`SELECT long_url FROM urls WHERE code = ?`, code).Scan(&longURL)
	return longURL, err
}

func lookupCodes(db *sql.DB, longURL string, limit int) ([]LookupResult, error) {
	h := urlHash(longURL)

	rows, err := db.Query(
		`SELECT code, created_at
		 FROM urls
		 WHERE long_url_hash = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		h, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LookupResult
	for rows.Next() {
		var r LookupResult
		if err := rows.Scan(&r.Code, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
