//go:build kmdai

package ai

import "strings"

// thinkSplitter moves a <think>…</think> block at the start of the streamed
// answer into the reasoning. llama.cpp streams reasoning apart, as
// reasoning_content, but other servers, or llama.cpp with
// --reasoning-format none, leave it inline. Reasoning stays enabled: in
// #44 turning it off made answers less accurate and the model easier to
// steer. Models write the block only before the answer, so a <think> later
// on is text (an answer about a document that mentions the tag). A block
// that is never closed holds the rest of the stream, so a model cut off
// while thinking shows no half-thought as its answer.
type thinkSplitter struct {
	state   thinkState
	pending string // text held back until it shows whether it is a tag
}

type thinkState int

const (
	thinkHead thinkState = iota // before any answer text: a block may start
	thinkIn                     // inside the block
	thinkDone                   // in the answer
)

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// split routes d's content to the answer or the reasoning. A tag split
// across pieces is held back until the next piece shows what it is.
func (t *thinkSplitter) split(d Delta) Delta {
	s := t.pending + d.Content
	t.pending = ""
	if t.state == thinkHead {
		head := strings.TrimLeft(s, " \t\r\n")
		switch {
		case strings.HasPrefix(head, thinkOpen):
			s, t.state = head[len(thinkOpen):], thinkIn
		case strings.HasPrefix(thinkOpen, head):
			t.pending = s // whitespace, or the start of the tag: wait
			return Delta{Reasoning: d.Reasoning}
		default:
			t.state = thinkDone
		}
	}
	if t.state == thinkDone {
		return Delta{Content: s, Reasoning: d.Reasoning}
	}
	i := strings.Index(s, thinkClose)
	if i < 0 {
		keep := partialTag(s, thinkClose)
		t.pending = s[len(s)-keep:]
		return Delta{Reasoning: d.Reasoning + s[:len(s)-keep]}
	}
	t.state = thinkDone
	return Delta{Content: s[i+len(thinkClose):], Reasoning: d.Reasoning + s[:i]}
}

// flush returns what split held back once the stream has ended: the start
// of a tag that never came is text after all.
func (t *thinkSplitter) flush() Delta {
	s := t.pending
	t.pending = ""
	if t.state == thinkIn {
		return Delta{Reasoning: s}
	}
	return Delta{Content: s}
}

// partialTag returns the length of the longest end of s that begins tag.
func partialTag(s, tag string) int {
	for n := min(len(s), len(tag)-1); n > 0; n-- {
		if strings.HasSuffix(s, tag[:n]) {
			return n
		}
	}
	return 0
}
