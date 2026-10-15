package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Cue struct {
	Start, End int64
	Text       string
}

// Align word timestamps to the punctuated utterance, keeping punctuation and
// original spelling. If alignment is incomplete, keep the semantic sentence.
func alignedWords(u Utterance) []Word {
	pos := 0
	var words []Word
	for _, w := range u.Words {
		text := strings.TrimSpace(w.Text)
		if text == "" {
			continue
		}
		i := strings.Index(u.Text[pos:], text)
		if i < 0 {
			return nil
		}
		at := pos + i
		if len(words) > 0 {
			words[len(words)-1].Text += u.Text[pos:at]
		} else if strings.TrimSpace(u.Text[:at]) != "" {
			return nil
		}
		end := at + len(text)
		if end > len(u.Text) || w.End <= w.Start {
			return nil
		}
		words = append(words, Word{Text: u.Text[at:end], Start: w.Start, End: w.End})
		pos = end
	}
	if len(words) > 0 {
		words[len(words)-1].Text += u.Text[pos:]
	}
	return words
}

func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			n += 2
		} else {
			n++
		}
	}
	return n
}
func wrapLine(text string, width int) string {
	var lines []string
	line := ""
	// Preserve word boundaries for spaced languages and character boundaries for CJK.
	var tokens []string
	word := ""
	for _, r := range strings.TrimSpace(text) {
		cjk := unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
		if cjk || unicode.IsSpace(r) {
			if word != "" {
				tokens = append(tokens, word)
				word = ""
			}
			if cjk {
				tokens = append(tokens, string(r))
			} else {
				tokens = append(tokens, " ")
			}
		} else {
			word += string(r)
		}
	}
	if word != "" {
		tokens = append(tokens, word)
	}
	for _, token := range tokens {
		if displayWidth(line+token) > width && strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimSpace(line))
			line = ""
		}
		if line == "" {
			token = strings.TrimLeftFunc(token, unicode.IsSpace)
		}
		line += token
	}
	if strings.TrimSpace(line) != "" {
		lines = append(lines, strings.TrimSpace(line))
	}
	return strings.Join(lines, "\n")
}
func sentenceBreak(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	return strings.ContainsRune(".!?。！？;；", r)
}
func makeCues(utterances []Utterance, c Config, offset int64) []Cue {
	var cues []Cue
	for _, u := range utterances {
		if strings.TrimSpace(u.Text) == "" || u.End <= u.Start {
			continue
		}
		words := alignedWords(u)
		if len(words) == 0 || (displayWidth(u.Text) <= c.LineWidth*2 && float64(u.End-u.Start) <= c.MaxCue*1000) {
			cues = append(cues, Cue{u.Start + offset, u.End + offset, wrapLine(u.Text, c.LineWidth)})
			continue
		}
		start := 0
		text := ""
		flush := func(end int) {
			cues = append(cues, Cue{words[start].Start + offset, words[end].End + offset, wrapLine(text, c.LineWidth)})
			start = end + 1
			text = ""
		}
		for i, w := range words {
			if i > start && (displayWidth(text+w.Text) > c.LineWidth*2 || float64(w.End-words[start].Start) > c.MaxCue*1000 || w.Start-words[i-1].End > 800) {
				flush(i - 1)
			}
			text += w.Text
			if sentenceBreak(w.Text) {
				flush(i)
			}
		}
		if start < len(words) {
			flush(len(words) - 1)
		}
	}
	return cues
}
func normalizeCues(cues []Cue, limit int64) []Cue {
	sort.SliceStable(cues, func(i, j int) bool { return cues[i].Start < cues[j].Start })
	var result []Cue
	for _, cue := range cues {
		if cue.Start < 0 {
			cue.Start = 0
		}
		if cue.End > limit {
			cue.End = limit
		}
		if cue.End <= cue.Start || cue.Start >= limit || strings.TrimSpace(cue.Text) == "" {
			continue
		}
		if n := len(result); n > 0 && result[n-1].End > cue.Start {
			result[n-1].End = cue.Start
			if result[n-1].End <= result[n-1].Start {
				result = result[:n-1]
			}
		}
		result = append(result, cue)
	}
	return result
}
func timestamp(ms int64) string {
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}
func writeSRT(w io.Writer, cues []Cue) error {
	for i, c := range cues {
		// Do not interpret model text as subtitle markup.
		text := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(c.Text)
		if _, err := fmt.Fprintf(w, "%d\n%s --> %s\n%s\n\n", i+1, timestamp(c.Start), timestamp(c.End), text); err != nil {
			return err
		}
	}
	return nil
}
