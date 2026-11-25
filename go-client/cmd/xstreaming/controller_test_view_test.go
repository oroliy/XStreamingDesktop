package main

import (
	"context"
	"slices"
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

func TestControllerTestPollReportsHotplugAndChanges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	shared := []controllerTestPad{{name: "pad"}}
	var got [][]controllerTestPad
	calls := 0
	pollControllerTest(ctx, func() []controllerTestPad {
		calls++
		switch calls {
		case 1, 2:
			return shared
		case 3:
			shared[0].state.Buttons = protocol.A
			return shared
		default:
			return nil
		}
	}, func(pads []controllerTestPad) {
		got = append(got, slices.Clone(pads))
		if len(got) == 3 {
			cancel()
		}
	}, time.Millisecond)
	if len(got) != 3 || calls != 4 || len(got[0]) != 1 || got[0][0].state.Buttons != 0 || got[1][0].state.Buttons != protocol.A || len(got[2]) != 0 {
		t.Fatalf("calls=%d, samples=%+v", calls, got)
	}
}

func TestControllerTestDiscardsLateUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &app{controllerTestCancel: cancel, controllerTestGeneration: 2}
	pads := []controllerTestPad{{name: "pad"}}
	a.applyControllerTestPads(ctx, 1, pads)
	if len(a.controllerTestPads) != 0 {
		t.Fatal("accepted old generation")
	}
	a.streamCancel = cancel
	a.applyControllerTestPads(ctx, 2, pads)
	if len(a.controllerTestPads) != 0 {
		t.Fatal("accepted update during stream")
	}
	a.streamCancel = nil
	cancel()
	a.applyControllerTestPads(ctx, 2, pads)
	if len(a.controllerTestPads) != 0 {
		t.Fatal("accepted canceled update")
	}
}
