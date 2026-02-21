package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"url-shortener/internal/cache"
	"url-shortener/internal/server"
	"url-shortener/internal/store"
)

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
	if err := store.EnsureSchema(db); err != nil {
		log.Fatal(err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
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
	mux.HandleFunc("/", s.Resolve)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
