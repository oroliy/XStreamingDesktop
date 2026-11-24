package main

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/session"
	"github.com/oroliy/XStreamingDesktop/go-client/settings"
	"github.com/pion/webrtc/v4"
	pionmedia "github.com/pion/webrtc/v4/pkg/media"
)

type testMicrophoneSession struct {
	start func(context.Context) (microphoneWriter, error)
	stops int
}

func TestMicrophoneControlVisibility(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &app{prefs: settings.Defaults(), streamCancel: cancel, streamCtx: ctx, micSession: &session.Session{}}
	tt := ui.NewTester(a.view, 1024, 768)
	if tt.HasText("Enable microphone") {
		t.Fatal("microphone offered before connection")
	}
	a.streamConnected = true
	tt.Frame()
	if got := tt.HasText("Enable microphone"); got != (runtime.GOOS == "linux") {
		t.Fatalf("unexpected microphone visibility: %v", got)
	}
	a.controlsHidden = true
	a.micStatus = "Microphone on."
	tt.Frame()
	if !tt.HasText("Microphone on.") {
		t.Fatal("capture status hidden with controls")
	}
}

func TestMicrophoneStopButtonCancelsCapture(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("microphone control is Linux-only")
	}
	streamCtx, stopStream := context.WithCancel(context.Background())
	defer stopStream()
	micCtx, stopMic := context.WithCancel(streamCtx)
	defer stopMic()
	a := &app{prefs: settings.Defaults(), streamCancel: stopStream, streamCtx: streamCtx, streamConnected: true, micSession: &session.Session{}, micCtx: micCtx, micCancel: stopMic}
	tt := ui.NewTester(a.view, 1024, 768)
	if err := tt.Click("Disable microphone"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(micCtx.Err(), context.Canceled) || !a.micStopping || a.micStatus != "Stopping microphone…" {
		t.Fatal("stop did not cancel capture")
	}
}

func (s *testMicrophoneSession) StartMicrophone(ctx context.Context) (microphoneWriter, error) {
	return s.start(ctx)
}
func (s *testMicrophoneSession) StopMicrophone() error { s.stops++; return nil }

func testMicrophoneTrack(t *testing.T) *webrtc.TrackLocalStaticSample {
	t.Helper()
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "mic", "test")
	if err != nil {
		t.Fatal(err)
	}
	return track
}

func TestMicrophoneCaptureStopsAndDetaches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &testMicrophoneSession{start: func(context.Context) (microphoneWriter, error) { return testMicrophoneTrack(t), nil }}
	active := 0
	source := "alsa_input.usb-example"
	err, stopErr := runMicrophone(ctx, ctx, source, s, func(ctx context.Context, args []string, send func(pionmedia.Sample) error) error {
		if !reflect.DeepEqual(args, []string{"-f", "pulse", "-i", source}) {
			t.Fatal(args)
		}
		for i := 0; i < 2; i++ {
			if err := send(pionmedia.Sample{Data: []byte{0xf8, 0xff, 0xfe}, Duration: 20 * time.Millisecond}); err != nil {
				t.Fatal(err)
			}
		}
		cancel()
		return ctx.Err()
	}, func() { active++ })
	if !errors.Is(err, context.Canceled) || stopErr != nil || active != 1 || s.stops != 1 {
		t.Fatalf("err=%v stop=%v active=%d detach=%d", err, stopErr, active, s.stops)
	}
}

func TestCancelDuringMicrophoneNegotiationSkipsCapture(t *testing.T) {
	micCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	track := testMicrophoneTrack(t)
	s := &testMicrophoneSession{start: func(context.Context) (microphoneWriter, error) {
		cancel()
		return track, nil
	}}
	err, stopErr := runMicrophone(context.Background(), micCtx, "default", s, func(context.Context, []string, func(pionmedia.Sample) error) error {
		t.Fatal("capture started after cancellation")
		return nil
	}, func() { t.Fatal("announced active after cancellation") })
	if !errors.Is(err, context.Canceled) || stopErr != nil || s.stops != 1 {
		t.Fatalf("err=%v stop=%v detach=%d", err, stopErr, s.stops)
	}
}

func TestSaveMicrophoneSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	a := &app{prefs: settings.Defaults(), settingsPath: path, micSourceDraft: "  alsa_input.usb-example  "}
	a.saveMicrophoneSource()
	saved, err := settings.Load(path)
	if err != nil || saved.MicrophoneSource != "alsa_input.usb-example" || a.prefs.MicrophoneSource != saved.MicrophoneSource {
		t.Fatalf("settings=%+v error=%v", saved, err)
	}
	a.micSourceDraft = "  "
	a.saveMicrophoneSource()
	if a.prefs.MicrophoneSource != "default" {
		t.Fatal("empty input did not restore the default source")
	}
	a.micSourceDraft = "bad\x00source"
	a.saveMicrophoneSource()
	if a.prefs.MicrophoneSource != "default" || a.status != "Invalid microphone input name." {
		t.Fatal("invalid input replaced the saved source")
	}
}

func TestMicrophoneSourceSettingsUI(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("microphone input setting is Linux-only")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	a := &app{prefs: settings.Defaults(), settingsPath: path}
	tt := ui.NewTester(a.view, 1024, 768)
	if err := tt.Click("PulseAudio input name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("alsa_input.example")
	if err := tt.Click("Save microphone input"); err != nil {
		t.Fatal(err)
	}
	saved, err := settings.Load(path)
	if err != nil || saved.MicrophoneSource != "alsa_input.example" {
		t.Fatalf("source=%q error=%v", saved.MicrophoneSource, err)
	}
}

func TestMicrophoneSourceOptionsPreserveSelection(t *testing.T) {
	got := microphoneSourceOptions([]string{"default", "virtual_mic", "virtual_mic"}, "disconnected_mic")
	want := []string{"default", "virtual_mic", "disconnected_mic"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestChooseDiscoveredMicrophoneSource(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("microphone input setting is Linux-only")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	a := &app{prefs: settings.Defaults(), settingsPath: path, micSourceDraft: "default", micSources: []string{"virtual_mic"}}
	tt := ui.NewTester(a.view, 1024, 768)
	if err := tt.Click("Available microphone inputs"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("virtual_mic"); err != nil {
		t.Fatalf("%v; visible: %v", err, tt.Texts())
	}
	if err := tt.Click("Save microphone input"); err != nil {
		t.Fatal(err)
	}
	saved, err := settings.Load(path)
	if err != nil || saved.MicrophoneSource != "virtual_mic" {
		t.Fatalf("source=%q error=%v", saved.MicrophoneSource, err)
	}
}
