package httpclient

import (
	"testing"
	"time"
)

func TestSSEParserMultiline(t *testing.T) {
	var p SSEParser
	at := time.Unix(1, 0)
	lines := []string{"event: message", `data: {"a":1}`, "data: second", "", "data: [DONE]", ""}
	var events []SSEEvent
	for _, line := range lines {
		if ev, ok := p.Line(line, at); ok {
			events = append(events, ev)
		}
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Event != "message" || events[0].Data != "{\"a\":1}\nsecond" {
		t.Fatalf("first event = %#v", events[0])
	}
	if events[1].Data != "[DONE]" {
		t.Fatalf("second event = %#v", events[1])
	}
}

func TestAguiIncremental(t *testing.T) {
	st := NewAguiState()
	feed := func(data string) {
		st.Apply(SSEEvent{Data: data, At: time.Unix(0, 0)})
	}
	feed(`{"type":"RUN_STARTED","runId":"r1","threadId":"t1"}`)
	feed(`{"type":"TEXT_MESSAGE_START","messageId":"m1","role":"assistant"}`)
	for _, s := range []string{"hel", "lo ", "world"} {
		feed(`{"type":"TEXT_MESSAGE_CONTENT","messageId":"m1","delta":"` + s + `"}`)
	}
	feed(`{"type":"TEXT_MESSAGE_END","messageId":"m1"}`)
	feed(`{"type":"TOOL_CALL_START","toolCallId":"tc1","toolCallName":"search"}`)
	feed(`{"type":"TOOL_CALL_ARGS","toolCallId":"tc1","delta":"{\"q\":\"go\"}"}`)
	feed(`{"type":"TOOL_CALL_RESULT","toolCallId":"tc1","content":"ok"}`)
	feed(`{"type":"RUN_FINISHED","runId":"r1"}`)

	snap := st.Snapshot()
	if snap.Status != AguiFinished || snap.RunID != "r1" || snap.ThreadID != "t1" {
		t.Fatalf("status = %#v", snap)
	}
	if len(snap.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(snap.Messages))
	}
	if snap.Messages[0].Text != "hello world" || !snap.Messages[0].Complete {
		t.Fatalf("text = %#v", snap.Messages[0])
	}
	if snap.Messages[1].ToolName != "search" || snap.Messages[1].Result != "ok" {
		t.Fatalf("tool = %#v", snap.Messages[1])
	}

	model := &ModelState{}
	model.Apply(SSEEvent{Data: `{"choices":[{"delta":{"content":"A"}}]}`})
	model.Apply(SSEEvent{Data: `{"choices":[{"delta":{"content":"B"}}]}`})
	if got := model.Text(); got != "AB" {
		t.Fatalf("model text = %q", got)
	}
}

func TestAguiInterrupt(t *testing.T) {
	st := NewAguiState()
	st.Apply(SSEEvent{Data: `{"type":"RUN_FINISHED","outcome":{"type":"interrupt","interrupts":[{"id":"i1","reason":"confirm","message":"continue?","metadata":{"toolName":"deploy","ui":{"type":"form","content":"{\"field\":\"env\"}"}}}]}}`})
	snap := st.Snapshot()
	if snap.Status != AguiInterrupted {
		t.Fatalf("status = %q", snap.Status)
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Kind != AguiInterrupt || snap.Messages[0].ToolName != "deploy" {
		t.Fatalf("messages = %#v", snap.Messages)
	}
	m := snap.Messages[0]
	if m.Reason != "confirm" || m.UIType != "form" || m.UIContent != `{"field":"env"}` {
		t.Fatalf("interrupt ui = %#v", m)
	}
}

func TestAguiMessagesSnapshot(t *testing.T) {
	st := NewAguiState()
	st.Apply(SSEEvent{Data: `{"type":"MESSAGES_SNAPSHOT","messages":[` +
		`{"id":"u1","role":"user","content":"hi"},` +
		`{"id":"a1","role":"assistant","content":[{"type":"text","text":"calling"},{"type":"tool-call","toolCallId":"tc1","toolName":"search","args":{"q":"go"}}]},` +
		`{"id":"t1","role":"tool","toolCallId":"tc1","content":"result-ok"}]}`})
	snap := st.Snapshot()
	if len(snap.Messages) != 4 {
		t.Fatalf("messages = %d: %#v", len(snap.Messages), snap.Messages)
	}
	if snap.Messages[0].Role != "user" || snap.Messages[0].Text != "hi" {
		t.Fatalf("user = %#v", snap.Messages[0])
	}
	if snap.Messages[1].Text != "calling" {
		t.Fatalf("assistant text = %#v", snap.Messages[1])
	}
	tc := snap.Messages[2]
	if tc.Kind != AguiToolCall || tc.ToolCallID != "tc1" || tc.ToolName != "search" || tc.Args != `{"q":"go"}` {
		t.Fatalf("tool call = %#v", tc)
	}
	res := snap.Messages[3]
	if res.Kind != AguiToolCall || res.ToolCallID != "tc1" || res.Result != "result-ok" {
		t.Fatalf("tool result = %#v", res)
	}

	// OpenAI-style tool_calls with a plain content string keep both.
	st.Apply(SSEEvent{Data: `{"type":"MESSAGES_SNAPSHOT","messages":[{"id":"a2","role":"assistant","content":"done",` +
		`"tool_calls":[{"id":"tc2","type":"function","function":{"name":"ping","arguments":"{}"}}]}]}`})
	snap = st.Snapshot()
	if len(snap.Messages) != 2 || snap.Messages[0].Text != "done" || snap.Messages[1].ToolName != "ping" {
		t.Fatalf("tool_calls snapshot = %#v", snap.Messages)
	}
}

func BenchmarkAguiApply(b *testing.B) {
	st := NewAguiState()
	st.Apply(SSEEvent{Data: `{"type":"TEXT_MESSAGE_START","messageId":"m","role":"assistant"}`})
	ev := SSEEvent{Data: `{"type":"TEXT_MESSAGE_CONTENT","messageId":"m","delta":"hello"}`}
	b.ReportAllocs()
	for b.Loop() {
		st.Apply(ev)
	}
}
