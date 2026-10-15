package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDefaultsOverridesAndTypos(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("GO_TRANS_API_KEY", "environment-key")
	template := strings.ReplaceAll(configTemplate, `"enable_itn": true`, `"enable_itn": false`)
	if err := os.WriteFile(path, []byte(template), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil || c.APIKey != "environment-key" || c.ITN || c.ResourceID != "volc.seedasr.auc" {
		t.Fatalf("bad config: %+v %v", c, err)
	}
	for _, invalid := range []string{`{"typo":true}`, `{"chunk_seconds":18001}`, `{"enable_itn":"false"}`, `{"language":"en-US"} {}`, `{"language":"en-US"} trailing`} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig(path); err == nil {
			t.Errorf("accepted invalid config: %s", invalid)
		}
	}
	if err := os.WriteFile(path, []byte(`{"api_key":"file-key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GO_TRANS_API_KEY", "")
	c, err = loadConfig(path)
	if err != nil || c.APIKey != "file-key" || !c.ITN || c.Chunk != 1800 {
		t.Fatalf("partial config defaults failed: %+v %v", c, err)
	}
}

func TestDefaultConfigLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if got := defaultConfigPath(); got != filepath.Join(home, ".config", "go-trans", "config.json") {
		t.Fatal(got)
	}
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if got := defaultConfigPath(); got != filepath.Join(xdg, "go-trans", "config.json") {
		t.Fatal(got)
	}
}

func TestConfigInitDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	run := func() error {
		root := newRoot()
		root.SetArgs([]string{"--config", path, "config", "init"})
		return root.Execute()
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = run(); err == nil {
		t.Fatal("existing configuration overwritten")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("configuration changed")
	}
}
