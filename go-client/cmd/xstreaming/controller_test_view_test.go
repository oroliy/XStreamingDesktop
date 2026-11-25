package main

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
)

func TestControllerTestDisplaysStateAndStops(t *testing.T) {
	stopped := false
	a := &app{controllerTestCancel: func() { stopped = true }, controllerTestPads: []controllerTestPad{{name: "Example pad", state: protocol.Gamepad{Buttons: protocol.A | protocol.DPadUp, LeftX: 0.5, RightTrigger: 1}}}}
	tt := ui.NewTester(a.view, 1024, 768)
	for _, label := range []string{"1. Example pad", "Buttons: A, ↑", "L: 0.50, 0.00  R: 0.00, 0.00", "LT: 0.00  RT: 1.00"} {
		if !tt.HasText(label) {
			t.Fatalf("missing %q: %v", label, tt.Texts())
		}
	}
	if err := tt.Click("Stop controller test"); err != nil {
		t.Fatal(err)
	}
	if !stopped || a.controllerTestCancel != nil || len(a.controllerTestPads) != 0 {
		t.Fatal("test did not stop and clear state")
	}
}

func TestControllerTestPollStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	pollControllerTest(ctx, func() []controllerTestPad { calls++; cancel(); return nil }, func([]controllerTestPad) { t.Fatal("published after cancellation") }, time.Millisecond)
	if calls != 1 {
		t.Fatalf("sampled %d times", calls)
	}
}

func TestControllerTestPollPublishesSamples(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	want := []controllerTestPad{{name: "pad", state: protocol.Gamepad{Buttons: protocol.B}}}
	var received []controllerTestPad
	pollControllerTest(ctx, func() []controllerTestPad { return want }, func(pads []controllerTestPad) {
		received = pads
		cancel()
	}, time.Millisecond)
	if len(received) != 1 || received[0] != want[0] {
		t.Fatalf("unexpected sample: %+v", received)
	}
}
