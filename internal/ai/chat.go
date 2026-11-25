//go:build kmdai

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Message is one turn of a conversation with the model. Images (#51) will
// need OpenAI's array-of-parts form of content instead of a string.
type Message struct {
	Role    string `json:"role"` // "system", "user" or "assistant"
	Content string `json:"content"`
}

// Request is a chat completion request. It has no field for tools or
// functions on purpose: the model is never given a capability (see
// docs/experimental/ai.md, "Threat model").
type Request struct {
	Model     string // "" lets the server use the model it has loaded
	Messages  []Message
	MaxTokens int // 0: the server's default
}

// Delta is a piece of the streamed answer. Both fields are the model's raw
// output, which a hostile document can steer: show them only through kmd's
// Markdown renderer (RenderAnswer), never print them as they are.
type Delta struct {
	Content   string // the answer, without any inline <think> block
	Reasoning string // the model's thinking: reasoning_content and <think> blocks
}

// ErrTruncated is returned after the last piece of an answer that the
// server cut off at Request.MaxTokens: what was streamed is incomplete.
var ErrTruncated = errors.New("the answer reached its length limit")

// errDone stops reading at the stream's [DONE] marker.
var errDone = errors.New("done")

// Chat streams a chat completion and calls on with each piece as it arrives.
// It returns when the server says the answer is done, on any error, or when
// on returns an error (which Chat then returns).
func (c *Client) Chat(ctx context.Context, req Request, on func(Delta) error) error {
	body, err := json.Marshal(struct {
		Model     string    `json:"model,omitempty"`
		Messages  []Message `json:"messages"`
		Stream    bool      `json:"stream"`
		MaxTokens int       `json:"max_tokens,omitempty"`
	}{req.Model, req.Messages, true, req.MaxTokens})
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, "POST", "/v1/chat/completions", body)
	if err != nil {
		return err
	}
	defer resp.Close()
	truncated := false
	var think thinkSplitter
	emit := func(d Delta) error {
		if d == (Delta{}) {
			return nil
		}
		return on(d)
	}
	err = readEvents(resp.body, c.lim.maxLine, func(data []byte) error {
		if string(data) == "[DONE]" {
			return errDone
		}
		return decodeEvent(data, &truncated, func(d Delta) error { return emit(think.split(d)) })
	})
	if errors.Is(err, errDone) {
		if ferr := emit(think.flush()); ferr != nil {
			return ferr
		}
	}
	switch {
	case errors.Is(err, errDone) && truncated:
		return ErrTruncated
	case errors.Is(err, errDone):
		return nil
	case errors.Is(err, io.EOF):
		err = errors.New("the answer ended before the server said it was done")
	}
	return c.explain(resp.ctx, err)
}

// decodeEvent decodes one streamed event and passes its text on. kmd never
// asks for more than one choice, so a fixed array keeps a hostile list of
// thousands from costing an allocation each: the extra ones are skipped.
func decodeEvent(data []byte, truncated *bool, on func(Delta) error) error {
	var ev struct {
		Choices [1]struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("the local AI server sent an invalid event: %w", err)
	}
	if len(ev.Error) > 0 && string(ev.Error) != "null" {
		return fmt.Errorf("the local AI server failed: %s", errorText(ev.Error))
	}
	ch := ev.Choices[0]
	if ch.FinishReason == "length" {
		*truncated = true
	}
	d := Delta{Content: ch.Delta.Content, Reasoning: ch.Delta.ReasoningContent}
	if d == (Delta{}) {
		return nil
	}
	return on(d)
}

// bodyError extracts {"error": …} from an error response's body.
func bodyError(body []byte) string {
	var v struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &v) != nil || len(v.Error) == 0 {
		return ""
	}
	return errorText(v.Error)
}

// errorText reads an OpenAI-style error, {"message": …} or a plain string,
// as printable text: it ends up on the terminal.
func errorText(raw json.RawMessage) string {
	var obj struct {
		Message string `json:"message"`
	}
	var s string
	if json.Unmarshal(raw, &obj) == nil && obj.Message != "" {
		s = obj.Message
	} else if json.Unmarshal(raw, &s) != nil {
		s = "unknown error"
	}
	return printable(s, longText)
}

// ServerInfo describes the model the server has loaded.
type ServerInfo struct {
	Model  string // "" when the server does not say
	Vision bool   // the model accepts images (llama.cpp's /props says so)
}

// maxInfoBody bounds /v1/models and /props, which include the chat template.
const maxInfoBody = 1 << 20

// Info asks the server for its model and whether it accepts images.
func (c *Client) Info(ctx context.Context) (ServerInfo, error) {
	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/v1/models", &models); err != nil {
		return ServerInfo{}, err
	}
	var info ServerInfo
	if len(models.Data) > 0 {
		info.Model = printable(models.Data[0].ID, longText)
	}
	var props struct {
		Modalities struct {
			Vision bool `json:"vision"`
		} `json:"modalities"`
	}
	// /props exists only on llama.cpp: any failure there means "no image
	// support known", not an error, unless the caller gave up.
	if err := c.getJSON(ctx, "/props", &props); err == nil {
		info.Vision = props.Modalities.Vision
	} else if ctx.Err() != nil {
		return ServerInfo{}, ctx.Err()
	}
	return info, nil
}

func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Close()
	b, err := io.ReadAll(&cappedReader{r: resp.body, left: maxInfoBody})
	if err != nil {
		return c.explain(resp.ctx, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("the local AI server sent invalid JSON for %s: %w", path, err)
	}
	return nil
}
