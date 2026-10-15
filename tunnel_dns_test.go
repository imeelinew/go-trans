package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTunnelDNSFiltersAnswers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "example.trycloudflare.com" || r.URL.Query().Get("type") != "A" || r.Header.Get("Accept") != "application/dns-json" {
			t.Error("wrong DNS request")
		}
		_, _ = w.Write([]byte(`{"Status":0,"Answer":[{"type":1,"data":"127.0.0.1"},{"type":1,"data":"10.0.0.1"},{"type":28,"data":"::1"},{"type":1,"data":"104.16.230.132"}]}`))
	}))
	defer server.Close()
	ips, err := resolveTunnel(context.Background(), server.URL, "example.trycloudflare.com")
	if err != nil || len(ips) != 1 || ips[0] != "104.16.230.132" {
		t.Fatalf("bad DNS response: %v %v", ips, err)
	}
}
