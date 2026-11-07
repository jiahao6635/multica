package agent

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCodexRawWebAndImageItems(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, item, tool string
		input            map[string]any
	}{
		{"legacy query", `{"type":"webSearch","id":"web-1","query":"Multica news"}`, "web_search", map[string]any{"query": "Multica news"}},
		{"search query", `{"type":"webSearch","id":"web-1","query":"news","action":{"type":"search","query":"Multica news"}}`, "web_search", map[string]any{"query": "news", "action": map[string]any{"type": "search", "query": "Multica news"}}},
		{"multiple queries", `{"type":"webSearch","id":"web-1","query":"news","action":{"type":"search","queries":["Multica news","Multica releases"]}}`, "web_search", map[string]any{"query": "news", "action": map[string]any{"type": "search", "queries": []any{"Multica news", "Multica releases"}}}},
		{"open page", `{"type":"webSearch","id":"web-1","query":"","action":{"type":"openPage","url":"https://example.com"}}`, "web_search", map[string]any{"query": "", "action": map[string]any{"type": "openPage", "url": "https://example.com"}}},
		{"find in page", `{"type":"webSearch","id":"web-1","query":"","action":{"type":"findInPage","url":"https://example.com","pattern":"release"}}`, "web_search", map[string]any{"query": "", "action": map[string]any{"type": "findInPage", "url": "https://example.com", "pattern": "release"}}},
		{"null action", `{"type":"webSearch","id":"web-1","query":"news","action":null}`, "web_search", map[string]any{"query": "news"}},
		{"pending search", `{"type":"webSearch","id":"web-1","query":"","action":{"type":"search","query":null,"queries":null}}`, "web_search", map[string]any{"query": "", "action": map[string]any{"type": "search", "query": nil, "queries": nil}}},
		{"image", `{"type":"imageView","id":"img-1","path":"/tmp/my image.png"}`, "view_image", map[string]any{"path": "/tmp/my image.png"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _, _ := newTestCodexClient(t)
			c.notificationProtocol = "raw"
			c.setThreadID("thr-main")
			var messages []Message
			c.onMessage = func(msg Message) { messages = append(messages, msg) }
			send := func(method, thread string) {
				c.handleLine(`{"jsonrpc":"2.0","method":"` + method + `","params":{"threadId":"` + thread + `","item":` + tt.item + `}}`)
			}
			send("item/started", "thr-other")
			send("item/completed", "thr-other")
			send("item/unknown", "thr-main")
			if len(messages) != 0 {
				t.Fatalf("unrelated notification emitted messages: %+v", messages)
			}
			send("item/started", "thr-main")
			if len(messages) != 1 {
				t.Fatalf("start must be visible before completion, got %+v", messages)
			}
			var item map[string]any
			if err := json.Unmarshal([]byte(tt.item), &item); err != nil {
				t.Fatal(err)
			}
			wantStart := Message{Type: MessageToolUse, Tool: tt.tool, CallID: item["id"].(string), Input: tt.input}
			if !reflect.DeepEqual(messages[0], wantStart) {
				t.Fatalf("start = %#v, want %#v", messages[0], wantStart)
			}
			send("item/completed", "thr-main")
			if len(messages) != 2 {
				t.Fatalf("expected pair, got %+v", messages)
			}
			wantEnd := Message{Type: MessageToolResult, Tool: tt.tool, CallID: wantStart.CallID}
			if !reflect.DeepEqual(messages[1], wantEnd) {
				t.Fatalf("completion = %#v, want %#v", messages[1], wantEnd)
			}
		})
	}
}

func TestCodexRawWebAndImageCompletionOnly(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ item, tool, id string }{
		{`{"type":"webSearch","id":"web-1","query":"news"}`, "web_search", "web-1"},
		{`{"type":"imageView","id":"img-1","path":"/tmp/image.png"}`, "view_image", "img-1"},
	} {
		t.Run(tt.tool, func(t *testing.T) {
			c, _, _ := newTestCodexClient(t)
			c.notificationProtocol = "raw"
			var messages []Message
			c.onMessage = func(msg Message) { messages = append(messages, msg) }
			c.handleLine(`{"jsonrpc":"2.0","method":"item/completed","params":{"item":` + tt.item + `}}`)
			want := []Message{{Type: MessageToolResult, Tool: tt.tool, CallID: tt.id}}
			if !reflect.DeepEqual(messages, want) {
				t.Fatalf("completion without start = %#v, want %#v", messages, want)
			}
			c.onMessage = nil
			c.handleLine(`{"jsonrpc":"2.0","method":"item/started","params":{"item":` + tt.item + `}}`)
			c.handleLine(`{"jsonrpc":"2.0","method":"item/completed","params":{"item":` + tt.item + `}}`)
		})
	}
}

func TestCodexRawWebAndImageInterleaved(t *testing.T) {
	t.Parallel()
	c, _, _ := newTestCodexClient(t)
	c.notificationProtocol = "raw"
	var messages []Message
	c.onMessage = func(msg Message) { messages = append(messages, msg) }
	lines := []string{
		`{"method":"item/started","params":{"item":{"type":"webSearch","id":"web-1","query":""}}}`,
		`{"method":"item/started","params":{"item":{"type":"imageView","id":"img-1","path":"/tmp/image.png"}}}`,
		`{"method":"item/completed","params":{"item":{"type":"imageView","id":"img-1","path":"/tmp/image.png"}}}`,
		`{"method":"item/completed","params":{"item":{"type":"webSearch","id":"web-1","query":"final query","action":{"type":"search","query":"final query"}}}}`,
	}
	for _, line := range lines {
		c.handleLine(line)
	}
	want := []Message{
		{Type: MessageToolUse, Tool: "web_search", CallID: "web-1", Input: map[string]any{"query": ""}},
		{Type: MessageToolUse, Tool: "view_image", CallID: "img-1", Input: map[string]any{"path": "/tmp/image.png"}},
		{Type: MessageToolResult, Tool: "view_image", CallID: "img-1"},
		{Type: MessageToolResult, Tool: "web_search", CallID: "web-1"},
	}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("interleaved calls = %#v, want %#v", messages, want)
	}
}

func TestCodexRawWebAndImageStartWithoutCompletion(t *testing.T) {
	t.Parallel()
	for _, item := range []string{
		`{"type":"webSearch","id":"web-1","query":"news"}`,
		`{"type":"imageView","id":"img-1","path":"/tmp/image.png"}`,
	} {
		c, _, _ := newTestCodexClient(t)
		c.notificationProtocol = "raw"
		var messages []Message
		c.onMessage = func(msg Message) { messages = append(messages, msg) }
		c.handleLine(`{"method":"item/started","params":{"item":` + item + `}}`)
		c.handleLine(`{"method":"turn/completed","params":{"turn":{"id":"turn-1","status":"cancelled"}}}`)
		if len(messages) != 1 || messages[0].Type != MessageToolUse {
			t.Fatalf("cancellation must not invent completion: %+v", messages)
		}
	}
}

func TestCodexRawWebAndImageInputRedaction(t *testing.T) {
	t.Parallel()
	c, _, _ := newTestCodexClient(t)
	c.notificationProtocol = "raw"
	var messages []Message
	c.onMessage = func(msg Message) { messages = append(messages, msg) }
	c.handleLine(`{"method":"item/started","params":{"item":{"type":"webSearch","id":"web-1","query":"Bearer secret-token-value","action":{"type":"findInPage","url":"https://example.com","pattern":"Bearer secret-token-value"}}}}`)
	if len(messages) != 1 {
		t.Fatalf("expected start, got %+v", messages)
	}
	want := map[string]any{"query": "Bearer [REDACTED]", "action": map[string]any{"type": "findInPage", "url": "https://example.com", "pattern": "Bearer [REDACTED]"}}
	if !reflect.DeepEqual(messages[0].Input, want) {
		t.Fatalf("input not redacted: %#v", messages[0].Input)
	}
}
