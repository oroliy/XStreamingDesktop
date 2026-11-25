package peer

import (
	"context"
	"encoding/json"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"testing"
	"time"
)

func TestSystemUITransactions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan protocol.Envelope, 2)
	requests := make(chan *SystemUIRequest, 2)
	m := newSystemUIRequests(ctx, func(e protocol.Envelope) error { sent <- e; return nil }, func(r *SystemUIRequest) { requests <- r })
	in := protocol.Envelope{Type: "TransactionStart", ID: "first", CV: "abc.4", Target: SystemUIVirtualKeyboard, Content: `{"TitleText":"Enter"}`}
	if !m.handle(in) {
		t.Fatal("request not handled")
	}
	r := <-requests
	if err := r.Complete(map[string]string{"Text": "hello"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.Done():
	default:
		t.Fatal("completed request still active")
	}
	out := <-sent
	var value map[string]string
	if out.Type != "TransactionComplete" || out.ID != "first" || out.CV != "abc.6" || json.Unmarshal([]byte(out.Content), &value) != nil || value["Text"] != "hello" {
		t.Fatalf("invalid response: %+v", out)
	}
	if err := r.Cancel(); err == nil {
		t.Fatal("accepted second reply")
	}
	in.ID = "second"
	if !m.handle(in) {
		t.Fatal("second request not handled")
	}
	r = <-requests
	if !m.handle(protocol.Envelope{Type: "SenderCancel", ID: "second"}) {
		t.Fatal("remote cancel not handled")
	}
	select {
	case <-r.Done():
	default:
		t.Fatal("remote cancellation did not close request")
	}
	if err := r.Complete(nil); err == nil {
		t.Fatal("answered canceled request")
	}
	select {
	case e := <-sent:
		t.Fatalf("sent after remote cancellation: %+v", e)
	default:
	}
}

func TestSystemUIShutdownAndUnsupported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	requests := make(chan *SystemUIRequest, 1)
	m := newSystemUIRequests(ctx, func(protocol.Envelope) error { t.Error("unexpected send"); return nil }, func(r *SystemUIRequest) { requests <- r })
	if m.handle(protocol.Envelope{Type: "TransactionStart", ID: "a", Target: "/unknown"}) {
		t.Fatal("accepted unsupported request")
	}
	if !m.handle(protocol.Envelope{Type: "Message", ID: "b", Target: SystemUIMessageDialog, Content: `{}`}) {
		t.Fatal("request not handled")
	}
	r := <-requests
	cancel()
	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown did not close request")
	}
	if err := r.Cancel(); err == nil {
		t.Fatal("canceled after shutdown")
	}
}
