package main

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/term"
)

type progress struct {
	mu   sync.Mutex
	tty  bool
	last string
}

func newProgress() *progress {
	return &progress{tty: term.IsTerminal(int(os.Stderr.Fd()))}
}
func (p *progress) update(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if text == p.last {
		return
	}
	p.last = text
	if p.tty {
		fmt.Fprintf(os.Stderr, "\r\x1b[2K%s", text)
	} else {
		fmt.Fprintln(os.Stderr, text)
	}
}
func (p *progress) finish() {
	if p.tty {
		fmt.Fprintln(os.Stderr)
	}
}
