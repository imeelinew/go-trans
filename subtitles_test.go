package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCuesPreserveLanguageAndPunctuation(t *testing.T) {
	for _, text := range []string{"Hello, how are you?", "你好，今天过得怎么样？", "こんにちは。", "مرحبا بك."} {
		cues := makeCues([]Utterance{{Text: text, Start: 100, End: 1500}}, defaults(), 60000)
		if len(cues) != 1 || cues[0].Start != 60100 || strings.ReplaceAll(cues[0].Text, "\n", "") != text {
			t.Fatalf("bad cue: %+v", cues)
		}
	}
}
func TestSplitAtTimedWords(t *testing.T) {
	c := defaults()
	c.MaxCue = 2
	u := Utterance{Text: "Hello, world. Welcome back!", Start: 100, End: 5000, Words: []Word{{Text: "Hello", Start: 100, End: 900}, {Text: "world", Start: 1000, End: 1700}, {Text: "Welcome", Start: 3000, End: 3900}, {Text: "back", Start: 4000, End: 4900}}}
	cues := makeCues([]Utterance{u}, c, 0)
	if len(cues) != 2 || cues[0].Text != "Hello, world." || cues[1].Text != "Welcome back!" || cues[1].Start != 3000 {
		t.Fatalf("bad split: %+v", cues)
	}
}
func TestUnalignedWordsPreserveSentence(t *testing.T) {
	c := defaults()
	c.MaxCue = 1
	u := Utterance{Text: "A complete sentence.", Start: 0, End: 5000, Words: []Word{{Text: "mismatch", Start: 0, End: 3000}}}
	cues := makeCues([]Utterance{u}, c, 0)
	if len(cues) != 1 || cues[0].Text != u.Text {
		t.Fatalf("lost text: %+v", cues)
	}
}
func TestNormalizeAndSRT(t *testing.T) {
	cues := normalizeCues([]Cue{{500, 2000, "Second"}, {-50, 700, "<First>"}, {3000, 4000, "Outside"}}, 1500)
	if len(cues) != 2 || cues[0].Start != 0 || cues[0].End != 500 || cues[1].End != 1500 {
		t.Fatalf("bad timestamps: %+v", cues)
	}
	var b bytes.Buffer
	if err := writeSRT(&b, cues); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "00:00:00,000 --> 00:00:00,500\n&lt;First&gt;") {
		t.Fatal(b.String())
	}
	if timestamp(3600123) != "01:00:00,123" {
		t.Fatal("timestamp overflow")
	}
}
