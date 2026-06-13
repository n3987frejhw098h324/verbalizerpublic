package main

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestIsLoopbackHostname(t *testing.T) {
	cases := map[string]bool{
		"localhost":   true,
		"LOCALHOST":   true,
		"127.0.0.1":   true,
		"127.0.0.55":  true,
		"::1":         true,
		"[::1]":       true,
		"":            false,
		"evil.com":    false,
		"10.0.0.5":    false,
		"192.168.1.4": false,
		"0.0.0.0":     false,
	}
	for in, want := range cases {
		if got := isLoopbackHostname(in); got != want {
			t.Errorf("isLoopbackHostname(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestHostAndOriginHelpers(t *testing.T) {
	if !hostIsLoopback("localhost:38073") {
		t.Error("hostIsLoopback should accept localhost:port")
	}
	if hostIsLoopback("evil.com:38073") {
		t.Error("hostIsLoopback should reject foreign host")
	}
	if !originIsLoopback("http://127.0.0.1:9000") {
		t.Error("originIsLoopback should accept loopback origin")
	}
	if originIsLoopback("https://evil.com") {
		t.Error("originIsLoopback should reject foreign origin")
	}
}

func TestLocalOnlyOverRealSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	if !ln.Addr().(*net.TCPAddr).IP.IsLoopback() {
		t.Fatalf("server bound to non-loopback address %v", ln.Addr())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", localOnly(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	defer srv.Close()

	addr := ln.Addr().String()

	do := func(host, origin string) int {
		req, err := http.NewRequest("GET", "http://"+addr+"/", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if host != "" {
			req.Host = host
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request (host=%q origin=%q): %v", host, origin, err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	port := addr[strings.LastIndex(addr, ":"):]

	cases := []struct {
		name   string
		host   string
		origin string
		want   int
	}{
		{"plugin-style: loopback host, no origin", "127.0.0.1" + port, "", http.StatusOK},
		{"localhost host", "localhost" + port, "", http.StatusOK},
		{"loopback origin allowed", "127.0.0.1" + port, "http://localhost:1234", http.StatusOK},
		{"DNS rebinding: foreign Host", "attacker.example", "", http.StatusForbidden},
		{"CSRF: foreign Origin", "127.0.0.1" + port, "http://attacker.example", http.StatusForbidden},
	}
	for _, c := range cases {
		if got := do(c.host, c.origin); got != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, got, c.want)
		}
	}
}
