package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveTunnel(t *testing.T) {
	if os.Getenv("GO_TRANS_NETWORK_TEST") != "1" {
		t.Skip("set GO_TRANS_NETWORK_TEST=1 for live Cloudflare check")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "chunk-0000.mp3")
	if err := os.WriteFile(file, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	base, cleanup, err := startTunnel(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	t.Log("tunnel", base)
	if _, err = publicAudio(ctx, defaults(), file, base); err != nil {
		t.Fatal(err)
	}
}
