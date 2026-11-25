package main

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/peer"
)

type fakeSystemRequest struct {
	target, payload string
	done            chan struct{}
	result          any
	canceled        bool
}

func (r *fakeSystemRequest) Done() <-chan struct{}    { return r.done }
func (r *fakeSystemRequest) TargetName() string       { return r.target }
func (r *fakeSystemRequest) Payload() string          { return r.payload }
func (r *fakeSystemRequest) Complete(value any) error { r.result = value; close(r.done); return nil }
func (r *fakeSystemRequest) Cancel() error            { r.canceled = true; close(r.done); return nil }

func TestSystemKeyboardPrompt(t *testing.T) {
	r := &fakeSystemRequest{target: peer.SystemUIVirtualKeyboard, payload: `{"TitleText":"Sign in","DescriptionText":"Enter password","DefaultText":"secret","InputScope":31,"MaxLength":20}`, done: make(chan struct{})}
	prompt, err := parseSystemPrompt(r)
	if err != nil || !prompt.password || prompt.maxLength != 20 {
		t.Fatalf("prompt=%+v error=%v", prompt, err)
	}
	a := &app{systemPrompt: prompt}
	tt := ui.NewTester(a.view, 1024, 768)
	if !tt.HasText("Sign in") || !tt.HasText("Enter password") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("Submit"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.result, map[string]string{"Text": "secret"}) || a.systemPrompt != nil {
		t.Fatalf("result=%v prompt=%v", r.result, a.systemPrompt)
	}
}

func TestSystemMessagePromptAndCancel(t *testing.T) {
	r := &fakeSystemRequest{target: peer.SystemUIMessageDialog, payload: `{"TitleText":"Choose","ContentText":"Continue?","CommandLabel1":"Yes","CommandLabel2":"No","CancelIndex":1}`, done: make(chan struct{})}
	prompt, err := parseSystemPrompt(r)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{systemPrompt: prompt}
	tt := ui.NewTester(a.view, 1024, 768)
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.result, map[string]int{"Result": 1}) || r.canceled {
		t.Fatalf("result=%v canceled=%v", r.result, r.canceled)
	}
}

func TestInvalidSystemPrompt(t *testing.T) {
	r := &fakeSystemRequest{target: peer.SystemUIVirtualKeyboard, payload: "bad", done: make(chan struct{})}
	if _, err := parseSystemPrompt(r); err == nil {
		t.Fatal("accepted malformed payload")
	}
}

func TestRemoteCancelRemovesPrompt(t *testing.T) {
	r := &fakeSystemRequest{target: peer.SystemUIVirtualKeyboard, payload: `{"TitleText":"Remote request"}`, done: make(chan struct{})}
	prompt, err := parseSystemPrompt(r)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{systemPrompt: prompt}
	tt := ui.NewTester(a.view, 1024, 768)
	if !tt.HasText("Remote request") {
		t.Fatal("prompt missing")
	}
	close(r.done)
	tt.Frame()
	if a.systemPrompt != nil || tt.HasText("Remote request") {
		t.Fatal("canceled prompt still visible")
	}
}

func TestSystemMessageFallbackCommands(t *testing.T) {
	r := &fakeSystemRequest{target: peer.SystemUIMessageDialog, payload: `{"Commands":["First",{"label":"Second","result":"7"}],"CancelIndex":"7"}`, done: make(chan struct{})}
	p, err := parseSystemPrompt(r)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.buttons, []systemButton{{"First", 0}, {"Second", 7}}) || p.cancelIndex == nil || *p.cancelIndex != 7 {
		t.Fatalf("prompt=%+v", p)
	}
}
