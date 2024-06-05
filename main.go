package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
)

func middlewareOne(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqDump, err := httputil.DumpRequestOut(r, true)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("REQUEST (CLIENT):\n%s", string(reqDump))
		next.ServeHTTP(w, r)
	})
}

func handler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	lookup(ctx, w, r)
}

func main() {
	// Setup groupcache
	registerGroup()

	mux := http.NewServeMux()
	finalHandler := http.HandlerFunc(handler)
	mux.Handle("/", middlewareOne(finalHandler))
	log.Print("starting server on :4000")

	err := http.ListenAndServe(":4000", mux)
	log.Fatal(err)
}
