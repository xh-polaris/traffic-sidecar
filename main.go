package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"k8s.io/utils/env"
)

var (
	Mode = env.GetString("XH_PROXY_MODE", "sidecar")
)

func main() {
	// 创建 HTTP/2 Cleartext 处理器
	h2s := &http2.Server{}
	h1s := &http.Server{
		Addr:    ":8080",
		Handler: h2c.NewHandler(http.HandlerFunc(handler), h2s),
	}
	http.NewServeMux()
	// 启用 HTTP/2 Cleartext 支持
	http2.ConfigureServer(h1s, h2s)
	// 启动服务器
	fmt.Println("Server is listening on port 8080")
	log.Fatal(h1s.ListenAndServe())
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Proxying request to:", r.URL.String())

	client := http.Client{
		Transport: &http2.Transport{
			// So http2.Transport doesn't complain the URL scheme isn't 'https'
			AllowHTTP: true,
			// Pretend we are dialing a TLS endpoint. (Note, we ignore the passed tls.Config)
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}
	req := &http.Request{
		Method:     r.Method,
		URL:        r.URL,
		Proto:      r.Proto,
		ProtoMajor: r.ProtoMajor,
		ProtoMinor: r.ProtoMinor,
		Header:     r.Header,
		Body:       r.Body,
		GetBody:    r.GetBody,
		Host:       r.Host,
	}
	r.URL.Scheme = "http"
	r.URL.Host = "platform-sts.xh-polaris:8080"

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Failed to send request: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// 将目标响应的状态码和头部复制到代理响应
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// 将目标响应的主体写入代理响应
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		fmt.Printf("Failed to copy response body: %v\n", err)
		return
	}
}
