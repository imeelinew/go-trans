package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDefaultsOverridesAndTypos(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.conf")
	t.Setenv("GO_TRANS_API_KEY", "environment-key")
	template := strings.ReplaceAll(configTemplate, "enable_itn = true", "enable_itn = false")
	if err := os.WriteFile(path, []byte(template), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil || c.APIKey != "environment-key" || c.ITN || c.ResourceID != "volc.seedasr.auc" {
		t.Fatalf("bad config: %+v %v", c, err)
	}
	if err := os.WriteFile(path, []byte(template+"typo = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("unknown key accepted")
	}
	if err := os.WriteFile(path, []byte("chunk_seconds = 18001\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("invalid chunk size accepted")
	}
}
