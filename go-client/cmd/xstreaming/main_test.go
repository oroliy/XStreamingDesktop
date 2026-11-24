package main

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/auth"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"github.com/oroliy/XStreamingDesktop/go-client/settings"
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
	a := &app{client: &streaming.Client{}, credentials: auth.NewSession("x"), consoles: []streaming.Console{{ServerID: "123", Name: "Living Room", ConsoleType: "XboxSeriesX", PowerState: "On"}}, titles: []streaming.Title{{TitleID: "game", Name: "Sample Game"}}}
	tt := ui.NewTester(a.view, 1024, 768)
	if !tt.HasText("Living Room (XboxSeriesX) — On") || !tt.HasText("Sample Game") {
		t.Fatal(tt.Texts())
	}
	a.streamCancel = cancel
	tt.Frame()
	if !tt.HasText("Stop stream") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("Enable keyboard controller"); err != nil {
		t.Fatal(err)
	}
	tt.KeyDown(ui.KeyW)
	enabled, state := a.keyboard.Snapshot()
	if !enabled || state.LeftY != -1 {
		t.Fatalf("keyboard: %+v", state)
	}
	tt.KeyUp(ui.KeyW)
	tt.KeyDown(ui.KeyEnter)
	_, enter := a.keyboard.Snapshot()
	if enter.Buttons&protocol.A == 0 || !a.keyboardEnabled {
		t.Fatal("Enter did not reach controller")
	}
	tt.KeyUp(ui.KeyEnter)
	tt.KeyDown(ui.KeyEscape)
	if a.keyboardEnabled {
		t.Fatal("Escape did not release keyboard")
	}
	tt.KeyUp(ui.KeyEscape)
	_, state = a.keyboard.Snapshot()
	if state.LeftY != 0 {
		t.Fatal("key release ignored")
	}
	if err := tt.Click("Stop stream"); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil || !a.busy || a.streamCancel != nil {
		t.Fatal("stop did not cancel active session")
	}
}

func TestCloudAccountWithoutHomeClient(t *testing.T) {
	a := &app{credentials: auth.NewSession("x"), cloudStatus: "3 games", titles: []streaming.Title{{TitleID: "game", Name: "Cloud title"}}}
	tt := ui.NewTester(a.view, 1024, 768)
	if tt.HasText("Sign in with Microsoft") || !tt.HasText("Sign out") || !tt.HasText("Cloud title") {
		t.Fatal(tt.Texts())
	}
}

func TestCloudGameSearch(t *testing.T) {
	a := &app{prefs: settings.Defaults(), credentials: auth.NewSession("x"), titles: []streaming.Title{{TitleID: "1", Name: "Halo Infinite"}, {TitleID: "2", Name: "Forza Horizon"}}}
	tt := ui.NewTester(a.view, 1024, 768)
	if !tt.HasText("Halo Infinite") || !tt.HasText("Forza Horizon") {
		t.Fatal(tt.Texts())
	}
	a.filter = "hAlO"
	tt.Frame()
	if !tt.HasText("Halo Infinite") || tt.HasText("Forza Horizon") {
		t.Fatal(tt.Texts())
	}
}
