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

func registerGroup() {
	// 3MB group
	group = groupcache.NewGroup("group", 3000000, groupcache.GetterFunc(func(ctx context.Context, key string, sink groupcache.Sink) error {
		originalRequest := ctx.Value("originalRequest").(*http.Request)

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
		fmt.Print("INSIDE GETTER FUNC")
		return nil
	}))
}

func lookup(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	ctx = context.WithValue(ctx, "originalRequest", r)
	cacheKey := getCacheKey(r)
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
