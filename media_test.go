package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func packetHashes(t *testing.T, path string) string {
	t.Helper()
	b, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-map", "0:v", "-map", "0:a", "-c", "copy", "-f", "streamhash", "-hash", "sha256", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("hash: %v %s", err, b)
	}
	return string(b)
}
func TestEmbedAndRemovePreservePackets(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " missing")
		}
	}
	for _, ext := range []string{".mkv", ".mp4"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "sample '日本語' "+ext)
			b, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=black:s=160x90:r=10:d=3", "-f", "lavfi", "-i", "sine=frequency=440:duration=3", "-c:v", "mpeg4", "-c:a", "aac", "-shortest", input).CombinedOutput()
			if err != nil {
				t.Fatalf("fixture: %v %s", err, b)
			}
			srt := filepath.Join(dir, "cues.srt")
			if err = os.WriteFile(srt, []byte("1\n00:00:00,100 --> 00:00:02,000\nHello. 你好。\n\n"), 0600); err != nil {
				t.Fatal(err)
			}
			originalHash := packetHashes(t, input)
			ctx := context.Background()
			media, err := probe(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			st, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			p := newProgress()
			if err = remux(ctx, input, input, srt, "und", st, media, p); err != nil {
				t.Fatal(err)
			}
			check, err := probe(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			if check.count("subtitle") != 1 {
				t.Fatal("subtitle missing")
			}
			if originalHash != packetHashes(t, input) {
				t.Fatal("audio/video packets changed during embed")
			}
			// An interrupted rewrite must leave the embedded source intact.
			st, err = os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if err = remux(canceled, input, input, "", "", st, check, p); err == nil {
				t.Fatal("canceled rewrite succeeded")
			}
			check, err = probe(ctx, input)
			if err != nil || check.count("subtitle") != 1 {
				t.Fatal("canceled rewrite changed input")
			}
			if err = removeVideo(ctx, input, ""); err != nil {
				t.Fatal(err)
			}
			check, err = probe(ctx, input)
			if err != nil || check.count("subtitle") != 0 {
				t.Fatal("remove failed")
			}
			if originalHash != packetHashes(t, input) {
				t.Fatal("audio/video packets changed during remove")
			}
			if err = removeVideo(ctx, input, ""); err != nil {
				t.Fatal("remove is not idempotent")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".go-trans-") || strings.HasSuffix(e.Name(), ".lock") {
					t.Fatalf("temporary file leaked: %s", e.Name())
				}
			}
		})
	}
}

func TestAudioLanguageUsesFirstTrack(t *testing.T) {
	m := Media{Streams: []Stream{{Type: "audio", Tags: map[string]string{"language": "eng"}}, {Type: "audio", Tags: map[string]string{"language": "zho"}}}}
	if audioLanguage(m) != "en-US" {
		t.Fatal("wrong audio language")
	}
	m.Streams[0].Tags["language"] = "und"
	if audioLanguage(m) != "" {
		t.Fatal("unknown tag should use detection")
	}
}
