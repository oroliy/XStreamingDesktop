package main

import (
	"context"
	"image"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/auth"
	"github.com/oroliy/XStreamingDesktop/go-client/session"
	"github.com/oroliy/XStreamingDesktop/go-client/settings"
)

func TestStreamControlsFitShortWindow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &app{
		streamCancel: cancel, streamCtx: ctx, streamConnected: true, inputReady: true,
		streamHomeID: "test", micSession: &session.Session{}, credentials: auth.NewSession("x"),
		frame: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 640, 360))),
	}
	tt := ui.NewTester(a.view, 1024, 400)
	for _, name := range []string{"Stop stream", "Press Xbox button", "Enable keyboard controller", "Send text"} {
		rect, ok := tt.Find(name)
		if !ok || rect.W <= 0 || rect.H <= 0 || rect.Y < 0 || rect.Y+rect.H > 400 {
			t.Errorf("%s is outside the window: %+v, found=%v", name, rect, ok)
		}
	}
}

func TestStreamControlsFitNarrowWindow(t *testing.T) {
	for _, language := range []string{"English", "简体中文"} {
		t.Run(language, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a := &app{
				streamCancel: cancel, streamCtx: ctx, accountCtx: ctx, streamConnected: true, inputReady: true,
				streamHomeID: "test", streamDone: make(chan error, 1), streamPowerOff: &atomic.Bool{},
				micSession: &session.Session{}, credentials: auth.NewSession("x"),
				frame: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 640, 360))), prefs: settings.Settings{UILanguage: language},
			}
			tt := ui.NewTester(a.view, 640, 480)
			for _, text := range []string{"Stop stream", "Disconnect and power off", "Press Xbox button", "Enable keyboard controller", "Send text"} {
				name := a.tr(text)
				rect, ok := tt.Find(name)
				if !ok || rect.W <= 0 || rect.H <= 0 || rect.X < 0 || rect.X+rect.W > 640 || rect.Y < 0 || rect.Y+rect.H > 480 {
					t.Errorf("%s is outside the window: %+v, found=%v", name, rect, ok)
				}
			}
		})
	}
}

func TestSettingsFitNarrowWindow(t *testing.T) {
	a := &app{prefs: settings.Defaults()}
	tt := ui.NewTester(a.view, 480, 480)
	labels := []string{"Resolution"}
	if microphoneSourceSettingsSupported(runtime.GOOS) {
		labels = append(labels, "Microphone input", "Refresh microphone inputs")
	}
	for _, text := range labels {
		r, ok := tt.Find(text)
		if !ok || r.W < 50 || r.H < 19 || r.X < 0 || r.X+r.W > 480 {
			t.Errorf("%s is clipped: %+v, found=%v", text, r, ok)
		}
	}
	if microphoneSourceSettingsSupported(runtime.GOOS) {
		tt.Scroll(240, 350, 0, 170)
	}
	for _, text := range []string{"Language", "Market"} {
		r, ok := tt.Find(text)
		if !ok || r.W < 40 || r.H < 19 || r.X < 0 || r.X+r.W > 480 || r.Y < 0 || r.Y+r.H > 480 {
			t.Errorf("%s is clipped after scrolling: %+v, found=%v", text, r, ok)
		}
	}
}
