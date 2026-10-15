package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const configTemplate = `# TOML configuration. GO_TRANS_API_KEY overrides api_key.
api_key = ""
resource_id = "volc.seedasr.auc"
# Empty means automatic language detection; e.g. "en-US", "zh-CN", "ja-JP".
language = ""
enable_itn = true
enable_punc = true
enable_ddc = false
enable_speaker_info = false
# Semantic sentence boundaries. Leave false for subtitles.
vad_segment = false
poll_interval_seconds = 3
timeout_seconds = 10800
# Long videos are processed in chunks (max 18000 seconds).
chunk_seconds = 1800
line_width = 42
max_cue_seconds = 7
# Local audio is served through a temporary Cloudflare tunnel.
# Alternatively, PUT each chunk to your own storage; {id} is a random object ID.
upload_put_url = ""
upload_get_url = ""
# Used only if local DNS cannot reach a new tunnel. Empty disables fallback.
tunnel_dns_url = "https://cloudflare-dns.com/dns-query"
`

type Config struct {
	TunnelDNS  string  `toml:"tunnel_dns_url"`
	APIKey     string  `toml:"api_key"`
	ResourceID string  `toml:"resource_id"`
	Language   string  `toml:"language"`
	ITN        bool    `toml:"enable_itn"`
	Punc       bool    `toml:"enable_punc"`
	DDC        bool    `toml:"enable_ddc"`
	Speaker    bool    `toml:"enable_speaker_info"`
	VAD        bool    `toml:"vad_segment"`
	Poll       int     `toml:"poll_interval_seconds"`
	Timeout    int     `toml:"timeout_seconds"`
	Chunk      int     `toml:"chunk_seconds"`
	LineWidth  int     `toml:"line_width"`
	MaxCue     float64 `toml:"max_cue_seconds"`
	PutURL     string  `toml:"upload_put_url"`
	GetURL     string  `toml:"upload_get_url"`
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
	return filepath.Join(base, "go-trans", "config.conf")
}
func loadConfig(path string) (Config, error) {
	c := defaults()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return c, err
	}
	if err == nil {
		if err = toml.NewDecoder(bytes.NewReader(b)).DisallowUnknownFields().Decode(&c); err != nil {
			return c, fmt.Errorf("config: %w", err)
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
