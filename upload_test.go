package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAudioHandlerIsolationAndRange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chunk-0000.mp3"), []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.conf"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(audioHandler(dir, "token"))
	defer server.Close()
	for _, path := range []string{"/", "/token/", "/token/config.conf", "/wrong/chunk-0000.mp3", "/token/../config.conf"} {
		r, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Body.Close()
		if r.StatusCode != 404 {
			t.Errorf("exposed %s", path)
		}
	}
	req, _ := http.NewRequest("GET", server.URL+"/token/chunk-0000.mp3", nil)
	req.Header.Set("Range", "bytes=2-4")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 206 || string(b) != "234" {
		t.Fatalf("range failed: %d %s", r.StatusCode, b)
	}
}
