package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Stream struct {
	Type        string            `json:"codec_type"`
	Codec       string            `json:"codec_name"`
	Tags        map[string]string `json:"tags"`
	Disposition struct {
		Default int `json:"default"`
	} `json:"disposition"`
}
type Media struct {
	Streams []Stream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func probe(ctx context.Context, path string) (Media, error) {
	var media Media
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", path)
	b, err := cmd.Output()
	if err != nil {
		return media, fmt.Errorf("cannot read video: %w", err)
	}
	if err = json.Unmarshal(b, &media); err != nil {
		return media, err
	}
	return media, nil
}
func (m Media) duration() (float64, error) {
	d, err := strconv.ParseFloat(m.Format.Duration, 64)
	if err != nil || math.IsNaN(d) || math.IsInf(d, 0) || d <= 0 {
		return 0, fmt.Errorf("video duration unavailable")
	}
	return d, nil
}
func (m Media) count(kind string) int {
	n := 0
	for _, s := range m.Streams {
		if s.Type == kind {
			n++
		}
	}
	return n
}

func runFFmpeg(ctx context.Context, args []string, duration float64, report func(string)) error {
	prefix := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-progress", "pipe:1", "-nostats"}
	cmd := exec.CommandContext(ctx, "ffmpeg", append(prefix, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	last := -1
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		if value, ok := strings.CutPrefix(scanner.Text(), "out_time_us="); ok && duration > 0 {
			us, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			percent := min(100, max(0, int(us/1e6/duration*100)))
			if percent != last {
				report(fmt.Sprintf("%d%%", percent))
				last = percent
			}
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		return fmt.Errorf("ffmpeg: %s", strings.TrimSpace(stderr.String()))
	}
	if scanErr != nil {
		return scanErr
	}
	report("100%")
	return nil
}

func mediaPaths(input, output string) (string, string, os.FileInfo, error) {
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			return "", "", nil, fmt.Errorf("%s missing", name)
		}
	}
	input, err := filepath.Abs(input)
	if err != nil {
		return "", "", nil, err
	}
	input, err = filepath.EvalSymlinks(input)
	if err != nil {
		return "", "", nil, err
	}
	info, err := os.Stat(input)
	if err != nil {
		return "", "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", "", nil, fmt.Errorf("input must be a regular video file")
	}
	ext := strings.ToLower(filepath.Ext(input))
	if ext != ".mkv" && ext != ".mp4" && ext != ".mov" && ext != ".m4v" {
		return "", "", nil, fmt.Errorf("supported formats: MKV, MP4, MOV, M4V")
	}
	if output == "" {
		output = input
	} else {
		output, err = filepath.Abs(output)
		if err != nil {
			return "", "", nil, err
		}
		dir, err := filepath.EvalSymlinks(filepath.Dir(output))
		if err != nil {
			return "", "", nil, err
		}
		output = filepath.Join(dir, filepath.Base(output))
		if output == input {
			return "", "", nil, fmt.Errorf("omit --output to replace the input")
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			return "", "", nil, fmt.Errorf("output already exists or is inaccessible")
		}
		if strings.ToLower(filepath.Ext(output)) != ext {
			return "", "", nil, fmt.Errorf("output must use the input container extension")
		}
	}
	return input, output, info, nil
}

func remux(ctx context.Context, input, output, srt, language string, source os.FileInfo, original Media, p *progress) error {
	temp, err := os.CreateTemp(filepath.Dir(output), ".go-trans-*"+filepath.Ext(output))
	if err != nil {
		return err
	}
	path := temp.Name()
	if err = temp.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	defer os.Remove(path)
	args := []string{"-i", input}
	if srt != "" {
		args = append(args, "-i", srt)
	}
	args = append(args, "-map", "0", "-map", "-0:s", "-map_metadata", "0", "-map_chapters", "0", "-c", "copy")
	if srt != "" {
		codec := "srt"
		if strings.ToLower(filepath.Ext(output)) != ".mkv" {
			codec = "mov_text"
		}
		if language == "" {
			language = "und"
		}
		args = append(args, "-map", "1:0", "-c:s", codec, "-disposition:s:0", "default", "-metadata:s:s:0", "title=go-trans", "-metadata:s:s:0", "handler_name=go-trans", "-metadata:s:s:0", "language="+language)
	}
	args = append(args, path)
	duration, err := original.duration()
	if err != nil {
		return err
	}
	if err = runFFmpeg(ctx, args, duration, func(percent string) {
		label := "[4/4] Embedding"
		if srt == "" {
			label = "Removing"
		}
		p.update(label + " · " + percent)
	}); err != nil {
		return err
	}
	if srt != "" {
		p.update("[4/4] Verifying")
	} else {
		p.update("Verifying")
	}
	checked, err := probe(ctx, path)
	if err != nil {
		return err
	}
	expected := 0
	if srt != "" {
		expected = 1
	}
	if checked.count("subtitle") != expected {
		return fmt.Errorf("subtitle verification failed")
	}
	var before, after []string
	for _, s := range original.Streams {
		if s.Type != "subtitle" {
			before = append(before, s.Type+":"+s.Codec)
		}
	}
	for _, s := range checked.Streams {
		if s.Type != "subtitle" {
			after = append(after, s.Type+":"+s.Codec)
		}
	}
	if strings.Join(before, ",") != strings.Join(after, ",") {
		return fmt.Errorf("stream verification failed")
	}
	for _, s := range checked.Streams {
		if s.Type == "subtitle" && s.Disposition.Default != 1 {
			return fmt.Errorf("default subtitle verification failed")
		}
	}
	current, err := os.Stat(input)
	if err != nil {
		return err
	}
	if !os.SameFile(source, current) || source.Size() != current.Size() || !source.ModTime().Equal(current.ModTime()) {
		return fmt.Errorf("input changed during processing")
	}
	if err = os.Chmod(path, source.Mode().Perm()); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if output == input {
		return os.Rename(path, output)
	}
	// Link without overwriting any output created during recognition.
	return os.Link(path, output)
}

func generateVideo(ctx context.Context, c Config, input, output string) error {
	input, output, source, err := mediaPaths(input, output)
	if err != nil {
		return err
	}
	media, err := probe(ctx, input)
	if err != nil {
		return err
	}
	if media.count("video") == 0 || media.count("audio") == 0 {
		return fmt.Errorf("video and audio streams required")
	}
	duration, err := media.duration()
	if err != nil {
		return err
	}
	if c.PutURL == "" {
		if _, err := exec.LookPath("cloudflared"); err != nil {
			return fmt.Errorf("cloudflared missing; install it or configure audio upload URLs")
		}
	}
	// Prevent concurrent jobs from replacing the same video.
	lock, err := os.OpenFile(input+".go-trans.lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("video is locked; remove %s.go-trans.lock if no job is running", input)
	}
	_ = lock.Close()
	defer os.Remove(input + ".go-trans.lock")
	work, err := os.MkdirTemp("", "go-trans-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	p := newProgress()
	defer p.finish()
	if c.Language == "" {
		c.Language = audioLanguage(media)
	}
	client := asrClient{config: c, base: apiBase, http: newHTTPClient()}
	chunks := int(math.Ceil(duration / float64(c.Chunk)))
	var cues []Cue
	base := ""
	for i := 0; i < chunks; i++ {
		offset := float64(i * c.Chunk)
		length := min(float64(c.Chunk), duration-offset)
		label := fmt.Sprintf("%d/%d", i+1, chunks)
		audio := filepath.Join(work, fmt.Sprintf("chunk-%04d.mp3", i))
		args := []string{"-ss", strconv.FormatFloat(offset, 'f', 3, 64), "-i", input, "-t", strconv.FormatFloat(length, 'f', 3, 64), "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "libmp3lame", "-b:a", "64k", audio}
		if err = runFFmpeg(ctx, args, length, func(percent string) { p.update("[1/4] Audio " + label + " · " + percent) }); err != nil {
			return err
		}
		p.update("[2/4] Sharing audio " + label)
		if c.PutURL == "" && base == "" {
			var cleanup func()
			base, cleanup, err = startTunnel(ctx, work)
			if err != nil {
				return err
			}
			defer cleanup()
		}
		url, err := publicAudio(ctx, c, audio, base)
		if err != nil {
			return err
		}
		p.update("[3/4] Submitting " + label)
		result, err := client.transcribe(ctx, url, func(status string) { p.update("[3/4] " + label + " · " + status) })
		if err != nil {
			return err
		}
		cues = append(cues, makeCues(result.Utterances, c, int64(math.Round(offset*1000)))...)
		_ = os.Remove(audio)
	}
	cues = normalizeCues(cues, int64(math.Round(duration*1000)))
	if len(cues) == 0 {
		return fmt.Errorf("no speech found; video unchanged")
	}
	srt := filepath.Join(work, "subtitles.srt")
	f, err := os.Create(srt)
	if err != nil {
		return err
	}
	err = writeSRT(f, cues)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = remux(ctx, input, output, srt, languageTag(c.Language), source, media, p); err != nil {
		return err
	}
	p.update(fmt.Sprintf("Done · %d cues · %s", len(cues), output))
	return nil
}

func removeVideo(ctx context.Context, input, output string) error {
	input, output, source, err := mediaPaths(input, output)
	if err != nil {
		return err
	}
	media, err := probe(ctx, input)
	if err != nil {
		return err
	}
	if media.count("subtitle") == 0 && output == input {
		fmt.Println("Done · No subtitle tracks")
		return nil
	}
	lock, err := os.OpenFile(input+".go-trans.lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("video is locked: %s.go-trans.lock", input)
	}
	_ = lock.Close()
	defer os.Remove(input + ".go-trans.lock")
	p := newProgress()
	defer p.finish()
	if err = remux(ctx, input, output, "", "", source, media, p); err != nil {
		return err
	}
	p.update("Done · Subtitles removed · " + output)
	return nil
}

func languageTag(language string) string {
	code := strings.Split(language, "-")[0]
	tags := map[string]string{"zh": "zho", "en": "eng", "ja": "jpn", "ko": "kor", "fr": "fra", "de": "deu", "es": "spa", "pt": "por", "ru": "rus", "it": "ita", "ar": "ara", "hi": "hin", "th": "tha", "vi": "vie", "id": "ind", "fil": "fil", "ms": "msa", "bn": "ben", "el": "ell", "nl": "nld", "tr": "tur", "pl": "pol", "ro": "ron", "ne": "nep", "uk": "ukr", "yue": "yue"}
	if tag, ok := tags[code]; ok {
		return tag
	}
	return "und"
}

// Use an explicit audio-language tag before asking the model to detect one.
func audioLanguage(media Media) string {
	tags := map[string]string{"eng": "en-US", "en": "en-US", "zho": "zh-CN", "chi": "zh-CN", "zh": "zh-CN", "jpn": "ja-JP", "ja": "ja-JP", "kor": "ko-KR", "ko": "ko-KR", "fra": "fr-FR", "fre": "fr-FR", "fr": "fr-FR", "deu": "de-DE", "ger": "de-DE", "de": "de-DE", "spa": "es-MX", "es": "es-MX", "por": "pt-BR", "pt": "pt-BR", "rus": "ru-RU", "ru": "ru-RU", "ita": "it-IT", "it": "it-IT", "ara": "ar-SA", "ar": "ar-SA", "tha": "th-TH", "th": "th-TH", "vie": "vi-VN", "vi": "vi-VN", "ind": "id-ID", "id": "id-ID", "fil": "fil-PH", "msa": "ms-MY", "may": "ms-MY", "ms": "ms-MY", "ben": "bn-BD", "bn": "bn-BD", "ell": "el-GR", "gre": "el-GR", "el": "el-GR", "nld": "nl-NL", "dut": "nl-NL", "nl": "nl-NL", "tur": "tr-TR", "tr": "tr-TR", "pol": "pl-PL", "pl": "pl-PL", "ron": "ro-RO", "rum": "ro-RO", "ro": "ro-RO", "nep": "ne-NP", "ne": "ne-NP", "ukr": "uk-UA", "uk": "uk-UA", "yue": "yue-CN"}
	for _, s := range media.Streams {
		if s.Type == "audio" {
			tag := strings.ToLower(s.Tags["language"])
			if language, ok := tags[tag]; ok {
				return language
			}
			if _, ok := tags[strings.Split(tag, "-")[0]]; ok && strings.Contains(tag, "-") {
				return s.Tags["language"]
			}
			return ""
		}
	}
	return ""
}
