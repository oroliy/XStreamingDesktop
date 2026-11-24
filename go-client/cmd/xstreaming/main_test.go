package main

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/auth"
	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
	"testing"
)

func TestInitialAndLoadingView(t *testing.T) {
	a := &app{}
	tt := ui.NewTester(a.view, 1024, 768)
	if !tt.HasText("XStreaming") || !tt.HasText("Sign in with Microsoft") {
		t.Fatal(tt.Texts())
	}
	a.busy = true
	a.status = "Requesting sign-in code…"
	tt.Frame()
	if tt.HasText("Sign in with Microsoft") || !tt.HasText(a.status) {
		t.Fatal(tt.Texts())
	}
	a.code = "ABCD"
	a.uri = "https://example.com"
	tt.Frame()
	if !tt.HasText("Code: ABCD") {
		t.Fatal(tt.Texts())
	}
}
func TestLibraryAndStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{client: &streaming.Client{}, credentials: auth.NewSession("x"), consoles: []streaming.Console{{ServerID: "123", Name: "Living Room", ConsoleType: "XboxSeriesX", PowerState: "On"}}, titles: []streaming.Title{{TitleID: "game", Name: "Sample Game"}}, streamCancel: cancel}
	tt := ui.NewTester(a.view, 1024, 768)
	if !tt.HasText("Living Room (XboxSeriesX) — On") || !tt.HasText("Sample Game") || !tt.HasText("Stop stream") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("Stop stream"); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil || !a.busy || a.streamCancel != nil {
		t.Fatal("stop did not cancel active session")
	}
}
