package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
)

func handler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	lookup(ctx, w, r)
}

func main() {
	// Setup groupcache
	registerGroup("sentinel-cogs.s3.amazonaws.com")
	peersConfig := os.Getenv("peers")
	peers := strings.Split(peersConfig, ",")
	pool := registerPeers(peers)

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(handler))
	mux.Handle("/_groupcache/", pool)
	log.Println(peers)
	log.Println("starting server on :4000")

	err := http.ListenAndServe(":4000", mux)
	log.Fatal(err)
}
