package main

import (
	"context"
	"errors"
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
	start func(context.Context) (*webrtc.TrackLocalStaticSample, error)
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

func (s *testMicrophoneSession) StartMicrophone(ctx context.Context) (*webrtc.TrackLocalStaticSample, error) {
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
	s := &testMicrophoneSession{start: func(context.Context) (*webrtc.TrackLocalStaticSample, error) { return testMicrophoneTrack(t), nil }}
	active := 0
	err, stopErr := runMicrophone(ctx, ctx, s, func(ctx context.Context, args []string, send func(pionmedia.Sample) error) error {
		if !reflect.DeepEqual(args, []string{"-f", "pulse", "-i", "default"}) {
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
	s := &testMicrophoneSession{start: func(context.Context) (*webrtc.TrackLocalStaticSample, error) {
		cancel()
		return track, nil
	}}
	err, stopErr := runMicrophone(context.Background(), micCtx, s, func(context.Context, []string, func(pionmedia.Sample) error) error {
		t.Fatal("capture started after cancellation")
		return nil
	}, func() { t.Fatal("announced active after cancellation") })
	if !errors.Is(err, context.Canceled) || stopErr != nil || s.stops != 1 {
		t.Fatalf("err=%v stop=%v detach=%d", err, stopErr, s.stops)
	}
}
