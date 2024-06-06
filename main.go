package main

import (
	"context"
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
	"github.com/spf13/viper"
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
	res, err := http.ReadResponse(reader, r)
	if err != nil {
		log.Fatal(err)
	}

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
				log.Panic(err)
			}
	
			log.Printf("Request handled by %s", me)
	
			// Rebuild HTTP request from cache key
			reader := bufio.NewReader(strings.NewReader(key))
			originalRequest, err := http.ReadRequest(reader)
			if err != nil {
				log.Panic(err)
			}
	
			// We can't have this set on client requests
			originalRequest.RequestURI = ""
	
			rawURL := "http://" + hostName
			if originalRequest.URL.Path != "" {
				rawURL = rawURL + originalRequest.URL.Path
			}
			fullUrl, err := url.Parse(rawURL)
			if err != nil {
				log.Panic(err)
			}
			originalRequest.URL = fullUrl
			originalRequest.Host = hostName
	
			client := http.Client{}
			res, err := client.Do(originalRequest)
			if err != nil {
				log.Panic(err)
			}
	
			// Write HTTP response out to bytes, store in cache
			var outBuf bytes.Buffer
			if err := res.Write(&outBuf); err != nil {
				log.Panic(err)
			}
			sink.SetBytes(outBuf.Bytes(), time.Time{})
			return nil
		},
	))	
}


func main() {
	config := viper.New()
	config.AutomaticEnv()
	proxyHostname := config.GetString("PROXY_HOSTNAME")
	cachePeers := config.GetString("GROUPCACHE_PEERS")
	if proxyHostname == "" {
		log.Fatal("Missing required env variable PROXY_HOSTNAME")
	}
	if cachePeers == "" {
		log.Fatal("Missing required env variable GROUPCACHE_PEERS")
	}

	// Setup groupcache
	newGroup(proxyHostname)
	peersList := strings.Split(cachePeers, ",")
	

	log.Printf("listening on %v", peersList[0])
	log.Printf("peers: %v", peersList)

	http.HandleFunc("/", http.HandlerFunc(proxyHandler))
	http.Handle("/_groupcache/", newPool(peersList))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
