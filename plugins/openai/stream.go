package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/usually-broken/lola/core/llm"
)

// Stream sends the conversation and hands back text as it arrives.
//
// The wire format is server-sent events: a line beginning "data: " carrying one
// JSON chunk, ending with "data: [DONE]". Text arrives as message.content deltas.
// Tool calls arrive as fragments — an argument string split across several chunks
// and indexed per call — so they are assembled here and returned whole.
func (p *Provider) Stream(ctx context.Context, req llm.Request, onText func(string) error) (llm.Response, error) {
	body, err := json.Marshal(chatRequest{
		Model:         req.Model,
		Messages:      toWireMessages(req.Messages),
		Tools:         toWireTools(req.Tools),
		Temperature:   req.Temperature,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	})
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body),
	)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai: send request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		// A provider that cannot stream answers with an error body, so read it
		// rather than reporting a bare status.
		var parsed chatResponse
		if err := json.NewDecoder(httpResp.Body).Decode(&parsed); err == nil && parsed.Error != nil {
			return llm.Response{}, fmt.Errorf("openai: %s", parsed.Error.Message)
		}
		return llm.Response{}, fmt.Errorf("openai: unexpected status %s", httpResp.Status)
	}

	var (
		content strings.Builder
		calls   = map[int]*streamCall{}
		order   []int
		tokens  int
	)

	scanner := bufio.NewScanner(httpResp.Body)
	// A single chunk with a large tool argument can exceed the default 64KB line
	// limit, so give the scanner room.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue // comments, blank separators, anything else on the wire
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return llm.Response{}, fmt.Errorf("openai: decode stream chunk: %w", err)
		}
		if chunk.Error != nil {
			return llm.Response{}, fmt.Errorf("openai: %s", chunk.Error.Message)
		}
		// The usage arrives on a chunk with no choices at all, which is why it is
		// read before the choices are checked.
		if chunk.Usage != nil && chunk.Usage.CompletionTokens > 0 {
			tokens = chunk.Usage.CompletionTokens
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		if delta == nil {
			continue
		}

		if delta.Content != "" {
			content.WriteString(delta.Content)
			if onText != nil {
				if err := onText(delta.Content); err != nil {
					return llm.Response{}, err
				}
			}
		}

		for _, tc := range delta.ToolCalls {
			call, seen := calls[tc.Index]
			if !seen {
				call = &streamCall{}
				calls[tc.Index] = call
				order = append(order, tc.Index)
			}
			if tc.ID != "" {
				call.id = tc.ID
			}
			if tc.Function.Name != "" {
				call.name += tc.Function.Name
			}
			call.args.WriteString(tc.Function.Arguments)
		}
	}

	if err := scanner.Err(); err != nil {
		return llm.Response{}, fmt.Errorf("openai: read stream: %w", err)
	}

	out := llm.Response{Content: content.String(), Tokens: tokens}
	for _, i := range order {
		call := calls[i]
		args := call.args.String()
		if args == "" {
			args = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
			ID:        call.id,
			Name:      call.name,
			Arguments: json.RawMessage(args),
		})
	}
	return out, nil
}

// streamChunk is one piece of a streamed reply. It is a separate type from
// chatResponse because a chunk carries a delta where a finished reply carries a
// message, and pretending they are the same hides that.
type streamChunk struct {
	Choices []struct {
		Delta *streamDelta `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// streamDelta is what a chunk adds to the reply so far.
type streamDelta struct {
	Content   string `json:"content"`
	ToolCalls []struct {
		Index    int    `json:"index"`
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

// streamCall is one tool call being assembled out of fragments.
type streamCall struct {
	id   string
	name string
	args strings.Builder
}

var _ llm.Streamer = (*Provider)(nil)
