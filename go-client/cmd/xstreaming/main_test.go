package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/auth"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"github.com/oroliy/XStreamingDesktop/go-client/settings"
	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
	"image"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
	if !tt.HasText("Living Room (XboxSeriesX) — On") || !tt.HasText("Sample Game") || !tt.HasText("Power off console") {
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

func TestVerificationPageButton(t *testing.T) {
	a := &app{busy: true, code: "CODE", uri: "https://microsoft.com/devicelogin"}
	tt := ui.NewTester(a.view, 1024, 768)
	if err := tt.Click("Open Microsoft sign-in page"); err != nil {
		t.Fatal(err)
	}
	urls := tt.OpenedURLs()
	if len(urls) != 1 || urls[0] != a.uri {
		t.Fatal(urls)
	}
	for _, invalid := range []string{"http://microsoft.com/devicelogin", "https://microsoft.com.evil.test/", "https://evil.test/?microsoft.com", "https://user@microsoft.com/", "file:///tmp/a"} {
		if verificationURLAllowed(invalid) {
			t.Fatalf("accepted %q", invalid)
		}
	}
}

func TestCancelSignIn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{busy: true, accountCtx: ctx, accountCancel: cancel, code: "ABCD", uri: "https://microsoft.com/devicelogin"}
	tt := ui.NewTester(a.view, 1024, 768)
	if err := tt.Click("Cancel sign-in"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(ctx.Err(), context.Canceled) || !a.cancelingLogin || !a.busy || a.code != "" || a.uri != "" {
		t.Fatalf("cancel failed: %+v", a)
	}
	if tt.HasText("Sign in with Microsoft") {
		t.Fatal("new sign-in allowed before cancellation finished")
	}
}

func TestChineseInterface(t *testing.T) {
	prefs := settings.Defaults()
	prefs.UILanguage = "简体中文"
	a := &app{prefs: prefs, status: "Connected; waiting for video…"}
	tt := ui.NewTester(a.view, 1024, 768)
	for _, text := range []string{"使用微软账号登录", "主机", "设置", "界面语言", "已连接，正在等待视频…"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
	if got := a.tr("Stream failed: network"); got != "串流启动失败：network" {
		t.Fatal(got)
	}
	if got := a.tr("Signed in. Found 2 console(s)."); got != "已登录。找到 2 台主机。" {
		t.Fatal(got)
	}
}

func TestQueueWaitTranslation(t *testing.T) {
	a := &app{prefs: settings.Settings{UILanguage: "简体中文"}}
	if got := a.tr("Estimated queue wait: 3 minute(s)."); got != "预计排队时间（分钟）：3" {
		t.Fatal(got)
	}
}

func TestStreamEndStatusReportsCleanupFailure(t *testing.T) {
	err := errors.New("service unavailable")
	if got := streamEndStatus(false, err); got != "Stream stopped; remote session cleanup failed: service unavailable" {
		t.Fatal(got)
	}
	if got := streamEndStatus(true, err); got != "Connection failed; remote session cleanup failed: service unavailable" {
		t.Fatal(got)
	}
	a := &app{prefs: settings.Settings{UILanguage: "简体中文"}}
	if got := a.tr(streamEndStatus(false, err)); got != "串流已停止，但远程会话清理失败：service unavailable" {
		t.Fatal(got)
	}
}

func TestNewCloudLoadCancelsPreviousRequest(t *testing.T) {
	var a app
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, firstGeneration := a.beginCloudLoad(parent)
	second, secondGeneration := a.beginCloudLoad(parent)
	if first.Err() != context.Canceled || second.Err() != nil || secondGeneration <= firstGeneration || a.cloudGeneration.Load() != secondGeneration {
		t.Fatal("cloud requests did not retain initiation order")
	}
	cancel()
	if second.Err() != context.Canceled {
		t.Fatal("account cancellation did not stop current request")
	}
}

func TestControllerHandshakeDelayAndRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	sessionDone := make(chan struct{})
	var inputOpen atomic.Bool
	notifications := make(chan bool, 2)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		monitorControllerReady(ctx, sessionDone, ready, inputOpen.Load, 10*time.Millisecond, time.Millisecond, func(waiting bool) { notifications <- waiting })
	}()
	select {
	case waiting := <-notifications:
		if !waiting {
			t.Fatal("expected delayed input notification")
		}
	case <-time.After(time.Second):
		t.Fatal("missing delayed input notification")
	}
	close(ready)
	inputOpen.Store(true)
	select {
	case waiting := <-notifications:
		if waiting {
			t.Fatal("expected recovery notification")
		}
	case <-time.After(time.Second):
		t.Fatal("missing recovery notification")
	}
	<-finished
}

func TestControllerMonitorStopsWithSession(t *testing.T) {
	done := make(chan struct{})
	close(done)
	monitorControllerReady(context.Background(), done, make(chan struct{}), func() bool { return false }, time.Second, time.Millisecond, func(bool) { t.Error("notification after session ended") })
}

func TestCloudAndExpiredSignInTranslations(t *testing.T) {
	a := &app{prefs: settings.Settings{UILanguage: "简体中文"}}
	if got := a.tr("Sign-in expired; please sign in again."); got != "登录信息已失效，请重新登录。" {
		t.Fatal(got)
	}
	if got := a.tr("Unable to connect to cloud games: connection failed"); got != "无法连接云游戏：connection failed" {
		t.Fatal(got)
	}
}

func TestCloudCatalogPaginationAndSearch(t *testing.T) {
	var titles []streaming.Title
	for i := range 85 {
		titles = append(titles, streaming.Title{TitleID: fmt.Sprintf("id-%d", i), Name: fmt.Sprintf("Game %03d", i)})
	}
	a := &app{prefs: settings.Defaults(), titles: titles}
	tt := ui.NewTester(a.view, 1024, 1200)
	if !tt.HasText("Game 000") || tt.HasText("Game 040") || !tt.HasText("Games 1–40 of 85") {
		t.Fatalf("unexpected first page: %v", tt.Texts())
	}
	if err := tt.Click("Next games"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Game 000") || !tt.HasText("Game 040") || !tt.HasText("Games 41–80 of 85") {
		t.Fatalf("unexpected second page: %v", tt.Texts())
	}
	if err := tt.Click("Search games"); err != nil {
		t.Fatal(err)
	}
	tt.Type("084")
	tt.Frame()
	if a.cloudPage != 0 || !tt.HasText("Game 084") || !tt.HasText("Games 1–1 of 1") {
		t.Fatalf("search did not reset page: page=%d %v", a.cloudPage, tt.Texts())
	}
}

func TestWaitForStreams(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	if waitForStreams(&wg, 10*time.Millisecond) {
		t.Fatal("returned before stream cleanup completed")
	}
	wg.Done()
	if !waitForStreams(&wg, time.Second) {
		t.Fatal("did not observe completed stream cleanup")
	}
}

func TestDecodedFrameRate(t *testing.T) {
	if fps := decodedFrameRate(10, 100, 1500*time.Millisecond); fps != 60 {
		t.Fatalf("got %v FPS", fps)
	}
	if decodedFrameRate(100, 10, time.Second) != 0 || decodedFrameRate(0, 10, 0) != 0 {
		t.Fatal("invalid frame rate interval")
	}
	prefs := settings.Defaults()
	prefs.UILanguage = "简体中文"
	a := &app{prefs: prefs, frame: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1))), decodedFPS: 59.5}
	tt := ui.NewTester(a.view, 1024, 768)
	if !tt.HasText("视频解码：59.5 帧/秒") {
		t.Fatal(tt.Texts())
	}
}

func TestCloudRegionOptionsKeepUnavailableSelection(t *testing.T) {
	got := cloudRegionOptions([]string{"East", "East", "West"}, "Gone")
	if !reflect.DeepEqual(got, []string{"Automatic", "East", "West", "Gone"}) {
		t.Fatal(got)
	}
}

func TestDisconnectAndPowerOffWaitsForCleanup(t *testing.T) {
	done := make(chan error, 1)
	result := make(chan error, 1)
	go func() { result <- waitForStreamCleanup(context.Background(), done) }()
	select {
	case err := <-result:
		t.Fatalf("returned before cleanup: %v", err)
	default:
	}
	failure := errors.New("remote stop failed")
	done <- failure
	if err := <-result; !errors.Is(err, failure) {
		t.Fatalf("lost cleanup error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForStreamCleanup(ctx, make(chan error)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestDisconnectAndPowerOffButtonForHomeStream(t *testing.T) {
	accountCtx, cancelAccount := context.WithCancel(context.Background())
	defer cancelAccount()
	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	flag := &atomic.Bool{}
	a := &app{prefs: settings.Defaults(), accountCtx: accountCtx, credentials: auth.NewSession("refresh"), streamCancel: cancelStream, streamHomeID: "console", streamDone: make(chan error), streamPowerOff: flag}
	tt := ui.NewTester(a.view, 1024, 768)
	if err := tt.Click("Disconnect and power off"); err != nil {
		t.Fatal(err)
	}
	if !flag.Load() || streamCtx.Err() == nil || !a.busy || a.streamCancel != nil || a.status != "Stopping stream before power-off…" {
		t.Fatal("combined action did not start cleanup")
	}
	cancelAccount()
}

func TestWakeBeforeStreamPreferencePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	a := &app{prefs: settings.Defaults(), settingsPath: path}
	tt := ui.NewTester(a.view, 1024, 768)
	if err := tt.Click("Wake standby console before streaming"); err != nil {
		t.Fatal(err)
	}
	saved, err := settings.Load(path)
	if err != nil || !saved.WakeBeforeStream {
		t.Fatalf("setting=%v error=%v", saved.WakeBeforeStream, err)
	}
}

func TestConsoleTextHiddenForKeyboardController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &app{prefs: settings.Defaults(), streamCtx: ctx, streamCancel: cancel, streamHomeID: "console", credentials: auth.NewSession("refresh")}
	tt := ui.NewTester(a.view, 1024, 768)
	if tt.HasText("Send text") {
		t.Fatal("text control shown before connection")
	}
	a.streamConnected = true
	tt.Frame()
	if !tt.HasText("Send text") {
		t.Fatal("home stream has no text control")
	}
	if err := tt.Click("Enable keyboard controller"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Send text") {
		t.Fatal("text control overlaps keyboard controller mode")
	}
}

func TestPowerOffRunsOnlyAfterSuccessfulCleanup(t *testing.T) {
	done := make(chan error, 1)
	called := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() {
		_, err := powerOffAfterCleanup(context.Background(), done, func(context.Context) error { called <- struct{}{}; return nil })
		result <- err
	}()
	select {
	case <-called:
		t.Fatal("power-off sent before cleanup")
	case <-time.After(20 * time.Millisecond):
	}
	done <- nil
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	default:
		t.Fatal("power-off not sent after cleanup")
	}

	failure := errors.New("cleanup failed")
	failed := make(chan error, 1)
	failed <- failure
	cleaned, err := powerOffAfterCleanup(context.Background(), failed, func(context.Context) error { t.Error("power-off sent after failed cleanup"); return nil })
	if cleaned || !errors.Is(err, failure) {
		t.Fatalf("cleaned=%v error=%v", cleaned, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ready := make(chan error, 1)
	ready <- nil
	cleaned, err = powerOffAfterCleanup(ctx, ready, func(context.Context) error { t.Error("power-off sent after cancellation"); return nil })
	if cleaned || !errors.Is(err, context.Canceled) {
		t.Fatalf("cleaned=%v error=%v", cleaned, err)
	}
}
