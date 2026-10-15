package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var tunnelPattern = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)
var chunkPattern = regexp.MustCompile(`^chunk-[0-9]+\.mp3$`)

// Only a random capability URL and the generated audio chunks are exposed.
// The source video and configuration are never served.
func audioHandler(dir, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/"+token+"/")
		if name == r.URL.Path || !chunkPattern.MatchString(name) || (r.Method != "GET" && r.Method != "HEAD") {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || !st.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, r, name, st.ModTime(), f)
	})
}

func startTunnel(ctx context.Context, dir string) (string, func(), error) {
	if _, err := exec.LookPath("cloudflared"); err != nil {
		return "", nil, fmt.Errorf("cloudflared missing; install it or configure upload_put_url and upload_get_url")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	token := requestID()
	server := &http.Server{Handler: audioHandler(dir, token), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	go func() { _ = server.Serve(listener) }()
	child, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(child, "cloudflared", "tunnel", "--config", os.DevNull, "--no-autoupdate", "--url", "http://"+listener.Addr().String(), "--protocol", "http2")
	reader, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err = cmd.Start(); err != nil {
		cancel()
		_ = server.Close()
		_ = reader.Close()
		_ = writer.Close()
		return "", nil, err
	}
	done := make(chan error, 1)
	go func() { err := cmd.Wait(); _ = writer.Close(); done <- err; close(done) }()
	found := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			if u := tunnelPattern.FindString(scanner.Text()); u != "" {
				select {
				case found <- u:
				default:
				}
			}
		}
	}()
	cleanup := func() { cancel(); _ = server.Close(); _ = reader.Close(); <-done }
	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()
	select {
	case base := <-found:
		return base + "/" + token, cleanup, nil
	case <-done:
		cleanup()
		return "", nil, fmt.Errorf("audio tunnel failed to start")
	case <-timer.C:
		cleanup()
		return "", nil, fmt.Errorf("audio tunnel timed out")
	case <-ctx.Done():
		cleanup()
		return "", nil, ctx.Err()
	}
}

func publicAudio(ctx context.Context, c Config, file, base string) (string, error) {
	if c.PutURL != "" {
		id := requestID()
		put := strings.ReplaceAll(c.PutURL, "{id}", id)
		get := strings.ReplaceAll(c.GetURL, "{id}", id)
		for _, s := range []string{put, get} {
			u, err := url.Parse(s)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
				return "", fmt.Errorf("invalid upload URL")
			}
		}
		f, err := os.Open(file)
		if err != nil {
			return "", err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, put, f)
		if err != nil {
			return "", err
		}
		req.ContentLength = st.Size()
		req.Header.Set("Content-Type", "audio/mpeg")
		client := &http.Client{Timeout: 10 * time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("audio upload failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("audio upload: HTTP %d", resp.StatusCode)
		}
		return get, nil
	}
	u := base + "/" + filepath.Base(file)
	// A quick-tunnel URL is printed before its route is ready. Verify it first.
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 8 * time.Second}
	last := "not ready"
	usingFallback := false
	defer func() { client.CloseIdleConnections() }()
	for {
		req, _ := http.NewRequestWithContext(timeout, http.MethodGet, u, nil)
		req.Header.Set("Range", "bytes=0-15")
		resp, err := client.Do(req)
		if err != nil {
			last = "connection failed"
			if !usingFallback && c.TunnelDNS != "" {
				client.CloseIdleConnections()
				client = tunnelHTTPClient(c.TunnelDNS)
				usingFallback = true
			}
		}
		if err == nil {
			last = fmt.Sprintf("HTTP %d", resp.StatusCode)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
				return u, nil
			}
		}
		select {
		case <-timeout.Done():
			return "", fmt.Errorf("audio URL unavailable (%s): %w", last, timeout.Err())
		case <-time.After(time.Second):
		}
	}
}
