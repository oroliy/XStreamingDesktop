package main

import (
	"context"
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/auth"
	"github.com/oroliy/XStreamingDesktop/go-client/session"
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
