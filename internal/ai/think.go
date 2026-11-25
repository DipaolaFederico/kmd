//go:build kmdai

package ai

import "strings"

// thinkSplitter moves <think>…</think> blocks out of the streamed answer
// into the reasoning. llama.cpp streams reasoning apart, as
// reasoning_content, but other servers, or llama.cpp with
// --reasoning-format none, leave it inline. Reasoning stays enabled: in
// #44 turning it off made answers less accurate and the model easier to
// steer. A block that is never closed holds the rest of the stream, so a
// model cut off while thinking shows no half-thought as its answer.
type thinkSplitter struct {
	in      bool   // inside a <think> block
	pending string // the end of the last piece, if it may begin a tag
}

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// split routes d's content to the answer or the reasoning. Tags split
// across pieces are held back until the next piece shows what they are.
func (t *thinkSplitter) split(d Delta) Delta {
	s := t.pending + d.Content
	t.pending = ""
	var answer, reasoning strings.Builder
	for s != "" {
		tag := thinkOpen
		if t.in {
			tag = thinkClose
		}
		out := &answer
		if t.in {
			out = &reasoning
		}
		if i := strings.Index(s, tag); i >= 0 {
			out.WriteString(s[:i])
			s, t.in = s[i+len(tag):], !t.in
			continue
		}
		keep := partialTag(s, tag)
		out.WriteString(s[:len(s)-keep])
		t.pending = s[len(s)-keep:]
		break
	}
	return Delta{Content: answer.String(), Reasoning: d.Reasoning + reasoning.String()}
}

// flush returns what split held back once the stream has ended: the start
// of a tag that never came is text after all.
func (t *thinkSplitter) flush() Delta {
	s := t.pending
	t.pending = ""
	if t.in {
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
