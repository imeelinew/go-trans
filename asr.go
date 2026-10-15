package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const apiBase = "https://openspeech.bytedance.com/api/v3/auc/bigmodel"

type Word struct {
	Text  string `json:"text"`
	Start int64  `json:"start_time"`
	End   int64  `json:"end_time"`
}
type Utterance struct {
	Text  string `json:"text"`
	Start int64  `json:"start_time"`
	End   int64  `json:"end_time"`
	Words []Word `json:"words"`
}
type Result struct {
	Text       string                     `json:"text"`
	Utterances []Utterance                `json:"utterances"`
	Additions  map[string]json.RawMessage `json:"additions"`
}
type recognition struct {
	Result Result `json:"result"`
}
type asrClient struct {
	config Config
	base   string
	http   *http.Client
}

func requestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func (a *asrClient) call(ctx context.Context, action, id string, payload any) ([]byte, string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/"+action, bytes.NewReader(b))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", a.config.APIKey)
	req.Header.Set("X-Api-Resource-Id", a.config.ResourceID)
	req.Header.Set("X-Api-Request-Id", id)
	if action == "submit" {
		req.Header.Set("X-Api-Sequence", "-1")
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("ASR %s: %w", action, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, "", err
	}
	code := resp.Header.Get("X-Api-Status-Code")
	if resp.StatusCode != 200 || (code != "20000000" && code != "20000001" && code != "20000002" && code != "20000003") {
		msg := strings.ReplaceAll(resp.Header.Get("X-Api-Message"), a.config.APIKey, "[redacted]")
		return nil, code, fmt.Errorf("ASR %s: HTTP %d / %s: %s (log %s)", action, resp.StatusCode, code, msg, resp.Header.Get("X-Tt-Logid"))
	}
	return body, code, nil
}

func (a *asrClient) transcribe(ctx context.Context, url string, report func(string)) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(a.config.Timeout)*time.Second)
	defer cancel()
	id := requestID()
	audio := map[string]any{"url": url, "format": "mp3", "rate": 16000, "channel": 1}
	if a.config.Language != "" {
		audio["language"] = a.config.Language
	}
	payload := map[string]any{"user": map[string]string{"uid": "go-trans"}, "audio": audio, "request": map[string]any{
		"model_name": "bigmodel", "enable_auto_lang": a.config.Language == "", "enable_itn": a.config.ITN, "enable_punc": a.config.Punc,
		"enable_ddc": a.config.DDC, "enable_speaker_info": a.config.Speaker, "show_utterances": true, "vad_segment": a.config.VAD,
	}}
	body, code, err := a.call(ctx, "submit", id, payload)
	if err != nil {
		return Result{}, err
	}
	if code != "20000000" {
		return Result{}, fmt.Errorf("ASR submit: unexpected status %s", code)
	}
	var submitted struct {
		TaskID string `json:"task_id"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &submitted); err != nil {
			return Result{}, fmt.Errorf("ASR submit response: %w", err)
		}
	}
	if submitted.TaskID != "" {
		id = submitted.TaskID
	}
	start := time.Now()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-time.After(time.Duration(a.config.Poll) * time.Second):
		}
		body, code, err := a.call(ctx, "query", id, map[string]any{})
		if err != nil {
			// Query is read-only: retry transient transport and server failures, never auth failures.
			if (code == "" || strings.HasPrefix(code, "55")) && failures < 3 && ctx.Err() == nil {
				failures++
				report("Retrying")
				continue
			}
			return Result{}, err
		}
		failures = 0
		switch code {
		case "20000000":
			var r recognition
			if err := json.Unmarshal(body, &r); err != nil {
				return Result{}, fmt.Errorf("ASR result: %w", err)
			}
			if strings.TrimSpace(r.Result.Text) != "" && len(r.Result.Utterances) == 0 {
				return Result{}, fmt.Errorf("ASR returned text without sentence timestamps")
			}
			return r.Result, nil
		case "20000003":
			return Result{}, nil
		case "20000001":
			report(fmt.Sprintf("Recognizing · %s", time.Since(start).Round(time.Second)))
		case "20000002":
			report(fmt.Sprintf("Queued · %s", time.Since(start).Round(time.Second)))
		}
	}
}
