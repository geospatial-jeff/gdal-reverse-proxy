package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"github.com/golang/groupcache"
)

func getCacheKey(r *http.Request) string {
	return r.Method + r.URL.String()
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

var group *groupcache.Group

func registerGroup(hostName string) {
	group = groupcache.NewGroup("group", 3000000, groupcache.GetterFunc(func(ctx context.Context, key string, sink groupcache.Sink) error {
		fmt.Println("Starting GetterFunc.")
		fmt.Println(key)
		decodedKey := strings.Replace(key, "|", "\n", -1)
		reader := bufio.NewReader(strings.NewReader(decodedKey))


		originalRequest, err := http.ReadRequest(reader)
		if err != nil {
			panic(err)
		}

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

		proxy := &httputil.ReverseProxy{
			Rewrite: func(r *httputil.ProxyRequest) {},
		}
		transport := proxy.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		res, err := transport.RoundTrip(originalRequest)
		if err != nil {
			log.Fatal(err)
		}

		var buf bytes.Buffer
		if err := res.Write(&buf); err != nil {
			log.Fatal(err)
		}

		sink.SetBytes(buf.Bytes())
		return nil
	}))
}

func registerPeers(peers []string) *groupcache.HTTPPool {
	pool := groupcache.NewHTTPPoolOpts(peers[0], nil)
	pool.Set(peers...)

	return pool
}


func lookup(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// ctx = context.WithValue(ctx, "originalRequest", r)

	// var inbuf bytes.Buffer
	// if err := r.Write(&inbuf); err != nil {
	// 	log.Fatal(err)
	// }

	var b = &bytes.Buffer{}
	if err := r.Write(b); err != nil {
		log.Fatal(err)
	}

	cacheKey := strings.Replace(b.String(), "\n", "|", -1)
	var data []byte
	if err := group.Get(ctx, cacheKey, groupcache.AllocatingByteSliceSink(&data)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	buf := bytes.NewBuffer(data)
	reader := bufio.NewReader(buf)
	res, _ := http.ReadResponse(reader, r)

	copyHeader(w.Header(), res.Header)
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
	res.Body.Close()
}
