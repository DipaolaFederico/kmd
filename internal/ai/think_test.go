//go:build kmdai

package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// splitAll feeds s to a thinkSplitter in pieces of n bytes.
func splitAll(s string, n int) (answer, reasoning string) {
	var t thinkSplitter
	for len(s) > 0 {
		k := min(n, len(s))
		d := t.split(Delta{Content: s[:k]})
		answer, reasoning, s = answer+d.Content, reasoning+d.Reasoning, s[k:]
	}
	d := t.flush()
	return answer + d.Content, reasoning + d.Reasoning
}

func TestThinkSplitter(t *testing.T) {
	for _, tc := range []struct{ in, answer, reasoning string }{
		{"Hello", "Hello", ""},
		{"<think>plan</think>Answer", "Answer", "plan"},
		{"\n  <think>plan</think>\n\nAnswer", "\n\nAnswer", "plan"},
		{"<think>never closed, then \x1b]52;c;x\x07", "", "never closed, then \x1b]52;c;x\x07"},
		{"<think>a <think> b</think>c", "c", "a <think> b"},
		{"<think>a</thin", "", "a</thin"},
		// only a leading block is reasoning: later tags are answer text
		{"The tag <think>x</think> marks reasoning.", "The tag <think>x</think> marks reasoning.", ""},
		{"<think>x</think>A<think>y</think>", "A<think>y</think>", "x"},
		{"x < y and <thin", "x < y and <thin", ""},
		{"<thin", "<thin", ""},
		{"  ", "  ", ""},
		{"</think>stray", "</think>stray", ""},
		{"<<think>>", "<<think>>", ""},
	} {
		// every piece size, so tags split at every byte are covered
		for n := 1; n <= len(tc.in); n++ {
			answer, reasoning := splitAll(tc.in, n)
			if answer != tc.answer || reasoning != tc.reasoning {
				t.Errorf("%q in pieces of %d = %q, %q; want %q, %q", tc.in, n, answer, reasoning, tc.answer, tc.reasoning)
			}
		}
	}
}

func TestChatStripsThink(t *testing.T) {
	pieces := []string{"<thi", "nk>The doc", "ument says</th", "ink>It is a", " list.<", "think>"}
	var events []string
	for _, p := range pieces {
		events = append(events, contentEvent(p))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, append(events, `{"choices":[{"delta":{},"finish_reason":"length"}]}`, `[DONE]`)...)
	}))
	defer srv.Close()
	answer, reasoning, err := collect(context.Background(), newClient(t, srv.URL), Request{})
	if !errors.Is(err, ErrTruncated) || answer != "It is a list.<think>" || reasoning != "The document says" {
		t.Fatalf("Chat = %q, %q, %v", answer, reasoning, err)
	}
}

func TestChatFlushesPartialTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, `{"choices":[{"delta":{"content":"a <thin"}}]}`, `[DONE]`)
	}))
	defer srv.Close()
	answer, _, err := collect(context.Background(), newClient(t, srv.URL), Request{})
	if err != nil || answer != "a <thin" {
		t.Fatalf("Chat = %q, %v", answer, err)
	}
}
