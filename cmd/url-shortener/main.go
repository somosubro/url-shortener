package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"url-shortener/internal/cache"
	"url-shortener/internal/server"
	"url-shortener/internal/store"
)

func getenv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func main() {
	dsn := getenv("MYSQL_DSN", "root:password@tcp(127.0.0.1:3306)/shortener?parseTime=true")
	redisAddr := getenv("REDIS_ADDR", "127.0.0.1:6379")

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	if err := store.EnsureSchema(db); err != nil {
		log.Fatal(err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	{
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Println("redis ping failed (continuing without cache):", err)
			rdb = nil
		}
	}

	s := server.NewServer(store.NewDBStore(db), cache.New(rdb))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.Health)
	mux.HandleFunc("/shorten", s.Shorten)
	mux.HandleFunc("/lookup", s.Lookup)
	mux.HandleFunc("/", s.Resolve)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
