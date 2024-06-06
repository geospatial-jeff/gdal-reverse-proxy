package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"bytes"
	"bufio"
	"strings"
	"time"
	"io"
	"github.com/mailgun/groupcache/v2"
)


func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	// Serialize request to string (cache key)
	var b = &bytes.Buffer{}
	if err := r.Write(b); err != nil {
		log.Fatal(err)
	}

	// Hit the cache
	var data []byte
	if err := group.Get(r.Context(), b.String(), groupcache.AllocatingByteSliceSink(&data)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Serialize bytes into http response
	buf := bytes.NewBuffer(data)
	reader := bufio.NewReader(buf)
	res, _ := http.ReadResponse(reader, r)

	copyHeader(w.Header(), res.Header)
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
	res.Body.Close()
}	

func newPool(peers []string) *groupcache.HTTPPool {
	pool := groupcache.NewHTTPPoolOpts(peers[0], nil)
	pool.Set(peers...)

	return pool
}

var group *groupcache.Group

func newGroup(hostName string) {
	group = groupcache.NewGroup("requests", 3<<20, groupcache.GetterFunc(
		func(_ context.Context, key string, sink groupcache.Sink) error {
			me, err := os.Hostname()
			if err != nil {
				panic("Get Hostname: " + err.Error())
			}
	
			log.Printf("Request handled by %s", me)
	
			// Rebuild HTTP request from cache key
			reader := bufio.NewReader(strings.NewReader(key))
			originalRequest, err := http.ReadRequest(reader)
			if err != nil {
				panic(err)
			}
	
			// We can't have this set on client requests
			originalRequest.RequestURI = ""
	
			rawURL := "http://" + hostName
			if originalRequest.URL.Path != "" {
				rawURL = rawURL + originalRequest.URL.Path
			}
			fullUrl, err := url.Parse(rawURL)
			if err != nil {
				log.Fatal(err)
			}
			originalRequest.URL = fullUrl
			originalRequest.Host = hostName
	
			client := http.Client{}
			res, err := client.Do(originalRequest)
			if err != nil {
				panic(err)
			}
	
			// Write HTTP response out to bytes, store in cache
			var outBuf bytes.Buffer
			if err := res.Write(&outBuf); err != nil {
				log.Fatal(err)
			}
			sink.SetBytes(outBuf.Bytes(), time.Time{})
			return nil
		},
	))	
}

func getPeers() []string {
	me, err := os.Hostname()
	if err != nil {
		panic("Get Hostname: " + err.Error())
	}

	me = fmt.Sprintf("http://%s:8080", me)

	peers := []string{
		"http://app1:8080",
		"http://app2:8080",
		"http://app3:8080",
	}

	for i, v := range peers {
		if v == me {
			peers = append(peers[:i], peers[i+1:]...)
		}
	}

	return append([]string{me}, peers...)
}

func main() {
	proxyHostname := os.Getenv("PROXY_HOSTNAME")
	newGroup(proxyHostname)

	peers := getPeers()

	log.Printf("listening on %v", peers[0])
	log.Printf("peers: %v", peers)

	http.HandleFunc("/", http.HandlerFunc(proxyHandler))
	http.Handle("/_groupcache/", newPool(peers))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
