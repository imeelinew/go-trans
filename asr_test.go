package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecognitionContract(t *testing.T) {
	calls := 0
	var id string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" || r.Header.Get("X-Api-Resource-Id") != "volc.seedasr.auc" {
			t.Error("wrong authentication")
		}
		w.Header().Set("X-Api-Status-Code", "20000000")
		switch r.URL.Path {
		case "/submit":
			id = r.Header.Get("X-Api-Request-Id")
			if len(id) != 36 || r.Header.Get("X-Api-Sequence") != "-1" {
				t.Error("invalid request headers")
			}
			var body struct {
				Audio   map[string]any `json:"audio"`
				Request map[string]any `json:"request"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Request["enable_auto_lang"] != true || body.Request["show_utterances"] != true || body.Request["vad_segment"] != false || body.Request["model_name"] != "bigmodel" {
				t.Errorf("wrong subtitle request: %#v", body.Request)
			}
			if _, ok := body.Audio["language"]; ok {
				t.Error("auto mode must not force a language")
			}
			if body.Audio["format"] != "mp3" {
				t.Error("wrong audio format")
			}
			_, _ = w.Write([]byte(`{"task_id":"returned-task"}`))
		case "/query":
			if r.Header.Get("X-Api-Request-Id") != "returned-task" {
				t.Error("did not use returned task ID")
			}
			calls++
			if calls == 1 {
				w.Header().Set("X-Api-Status-Code", "20000002")
				_, _ = w.Write([]byte(`{}`))
				return
			}
			_, _ = w.Write([]byte(`{"result":{"text":"Hello.","utterances":[{"text":"Hello.","start_time":100,"end_time":1000}]}}`))
		default:
			t.Error("unexpected endpoint")
		}
	}))
	defer server.Close()
	c := defaults()
	c.APIKey = "test-key"
	c.Poll = 1
	client := asrClient{config: c, base: server.URL, http: server.Client()}
	result, err := client.transcribe(context.Background(), "https://example.com/audio.mp3", func(string) {})
	if err != nil || len(result.Utterances) != 1 || calls != 2 || id == "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
func TestAuthFailureAndCancellation(t *testing.T) {
	n := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.Header().Set("X-Api-Status-Code", "45000030")
		w.Header().Set("X-Api-Message", "test-key not granted")
		w.WriteHeader(403)
	}))
	defer server.Close()
	c := defaults()
	c.APIKey = "test-key"
	client := asrClient{config: c, base: server.URL, http: server.Client()}
	_, err := client.transcribe(context.Background(), "https://example.com/audio.mp3", func(string) {})
	if err == nil || !strings.Contains(err.Error(), "45000030") || strings.Contains(err.Error(), "test-key") || n != 1 {
		t.Fatalf("unexpected auth handling: %v (%d calls)", err, n)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err = client.transcribe(ctx, "https://example.com/audio.mp3", func(string) {})
	if err == nil || time.Since(start) > time.Second {
		t.Fatal("cancellation failed")
	}
}
func TestMalformedResultDoesNotSucceed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Api-Status-Code", "20000000")
		if r.URL.Path == "/submit" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"text":"Untimed text"}}`))
	}))
	defer server.Close()
	c := defaults()
	c.Poll = 1
	a := asrClient{config: c, base: server.URL, http: server.Client()}
	if _, err := a.transcribe(context.Background(), "https://example.com/audio.mp3", func(string) {}); err == nil {
		t.Fatal("untimed text accepted")
	}
}
