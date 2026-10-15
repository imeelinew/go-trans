package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

//go:embed config.example.json
var configTemplate string

type Config struct {
	TunnelDNS  string  `json:"tunnel_dns_url"`
	APIKey     string  `json:"api_key"`
	ResourceID string  `json:"resource_id"`
	Language   string  `json:"language"`
	ITN        bool    `json:"enable_itn"`
	Punc       bool    `json:"enable_punc"`
	DDC        bool    `json:"enable_ddc"`
	Speaker    bool    `json:"enable_speaker_info"`
	VAD        bool    `json:"vad_segment"`
	Poll       int     `json:"poll_interval_seconds"`
	Timeout    int     `json:"timeout_seconds"`
	Chunk      int     `json:"chunk_seconds"`
	LineWidth  int     `json:"line_width"`
	MaxCue     float64 `json:"max_cue_seconds"`
	PutURL     string  `json:"upload_put_url"`
	GetURL     string  `json:"upload_get_url"`
}

func defaults() Config {
	return Config{TunnelDNS: "https://cloudflare-dns.com/dns-query", ResourceID: "volc.seedasr.auc", ITN: true, Punc: true, Poll: 3, Timeout: 10800, Chunk: 1800, LineWidth: 42, MaxCue: 7}
}
func defaultConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "go-trans", "config.json")
}
func loadConfig(path string) (Config, error) {
	c := defaults()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return c, err
	}
	if err == nil {
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&c); err != nil {
			return c, fmt.Errorf("config: %w", err)
		}
		if err = decoder.Decode(new(any)); err != io.EOF {
			return c, fmt.Errorf("config: expected a single JSON object")
		}
	}
	if key := os.Getenv("GO_TRANS_API_KEY"); key != "" {
		c.APIKey = key
	}
	if c.APIKey == "" {
		return c, fmt.Errorf("missing API key; run 'go-trans config init' and set api_key, or set GO_TRANS_API_KEY")
	}
	if c.ResourceID != "volc.seedasr.auc" {
		return c, fmt.Errorf("resource_id must be volc.seedasr.auc (recording recognition 2.0)")
	}
	if c.Poll < 1 || c.Timeout < c.Poll || c.Timeout > int((24*time.Hour).Seconds()) {
		return c, fmt.Errorf("invalid polling or timeout settings")
	}
	if c.Chunk < 1 || c.Chunk > 18000 {
		return c, fmt.Errorf("chunk_seconds must be 1..18000")
	}
	if c.LineWidth < 10 || c.LineWidth > 100 || c.MaxCue < 1 || c.MaxCue > 30 {
		return c, fmt.Errorf("invalid subtitle settings")
	}
	if (c.PutURL == "") != (c.GetURL == "") {
		return c, fmt.Errorf("set both upload_put_url and upload_get_url")
	}
	return c, nil
}
