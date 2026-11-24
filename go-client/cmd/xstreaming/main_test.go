package main

import (
	"context"
	"errors"
	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/auth"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"github.com/oroliy/XStreamingDesktop/go-client/settings"
	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
	"image"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	tt.SetFocused(false)
	_, unfocused := a.keyboard.Snapshot()
	if unfocused.LeftY != 0 {
		t.Fatal("focus loss left a key held")
	}
	tt.SetFocused(true)
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

func TestIPv6PreferencePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	a := &app{prefs: settings.Defaults(), settingsPath: path}
	tt := ui.NewTester(a.view, 1024, 768)
	if err := tt.Click("Prefer IPv6 candidates"); err != nil {
		t.Fatal(err)
	}
	saved, err := settings.Load(path)
	if err != nil || !saved.PreferIPv6 {
		t.Fatal(saved, err)
	}
}

func TestFetchHomeInitialAndRefresh(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v6/servers/home" || r.Header.Get("Authorization") != "Bearer test" {
			t.Error("invalid console request")
		}
		_, _ = w.Write([]byte(`{"results":[{"serverId":"id","deviceName":"Xbox","powerState":"On"}]}`))
	}))
	defer server.Close()
	tokens := 0
	provider := func(context.Context) (string, string, error) { tokens++; return "test", server.URL, nil }
	c, first, err := fetchHome(context.Background(), nil, provider)
	if err != nil || len(first) != 1 || first[0].PowerState != "On" {
		t.Fatal(first, err)
	}
	_, second, err := fetchHome(context.Background(), c, provider)
	if err != nil || len(second) != 1 || calls != 2 || tokens != 1 {
		t.Fatalf("calls=%d tokens=%d error=%v", calls, tokens, err)
	}
}
func TestFetchHomeTokenError(t *testing.T) {
	sentinel := errors.New("no token")
	_, _, err := fetchHome(context.Background(), nil, func(context.Context) (string, string, error) { return "", "", sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}

func TestPlayableTitlesExcludeMissingID(t *testing.T) {
	out := playableTitles([]streaming.Title{{TitleID: "playable", Name: "Game"}, {Name: "Missing ID"}})
	if len(out) != 1 || out[0].TitleID != "playable" {
		t.Fatal(out)
	}
}

func TestFirstVideoFrameUpdatesStatus(t *testing.T) {
	a := &app{status: "Connected; waiting for video…"}
	frame := image.NewRGBA(image.Rect(0, 0, 16, 16))
	a.showFrame(frame)
	if a.frame == nil || a.status != "Streaming video." {
		t.Fatal(a.status)
	}
	previous := a.frame
	a.status = "Stream error: audio failed"
	a.showFrame(frame)
	if a.frame != previous || a.status != "Stream error: audio failed" {
		t.Fatal("frame update obscured error or replaced bitmap")
	}
}
