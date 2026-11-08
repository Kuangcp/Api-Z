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
	st.Apply(SSEEvent{Data: `{"type":"RUN_FINISHED","outcome":{"type":"interrupt","interrupts":[{"id":"i1","reason":"confirm","message":"continue?","metadata":{"toolName":"deploy"}}]}}`})
	snap := st.Snapshot()
	if snap.Status != AguiInterrupted {
		t.Fatalf("status = %q", snap.Status)
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Kind != AguiInterrupt || snap.Messages[0].ToolName != "deploy" {
		t.Fatalf("messages = %#v", snap.Messages)
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
