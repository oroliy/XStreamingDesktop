package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/game/gamepad"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
)

type motorCall struct {
	pad         *gamepad.Pad
	left, right float64
}

func TestRumbleReplacementDisconnectAndClose(t *testing.T) {
	var mu sync.Mutex
	var calls []motorCall
	m := newRumbleManager(func(pad *gamepad.Pad, left, right float64) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, motorCall{pad, left, right})
	})
	pad := new(gamepad.Pad)
	m.play(0, pad, protocol.Vibration{LeftMotor: 1, Duration: 30 * time.Millisecond})
	m.play(0, pad, protocol.Vibration{RightMotor: 0.5, Duration: time.Second})
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	if len(calls) != 3 || calls[2].right != 0.5 {
		t.Errorf("old timer stopped replacement: %+v", calls)
	}
	mu.Unlock()
	m.reconcile(map[*gamepad.Pad]uint8{})
	mu.Lock()
	if len(calls) != 4 || calls[3].left != 0 || calls[3].right != 0 {
		t.Errorf("disconnect did not stop motors: %+v", calls)
	}
	mu.Unlock()
	m.play(1, pad, protocol.Vibration{LeftMotor: 0.7, Duration: time.Second})
	m.close()
	m.play(1, pad, protocol.Vibration{LeftMotor: 1, Duration: time.Second})
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 6 || calls[5].left != 0 || calls[5].right != 0 {
		t.Errorf("close did not stop motors: %+v", calls)
	}
}

func TestRumbleExpires(t *testing.T) {
	stopped := make(chan struct{}, 1)
	m := newRumbleManager(func(_ *gamepad.Pad, left, right float64) {
		if left == 0 && right == 0 {
			stopped <- struct{}{}
		}
	})
	defer m.close()
	m.play(0, new(gamepad.Pad), protocol.Vibration{LeftMotor: 1, Duration: 20 * time.Millisecond})
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("motors did not stop at end of effect")
	}
}

func TestDisabledVibrationDoesNotRegisterFeedback(t *testing.T) {
	if vibrationFeedback(nil, false) != nil {
		t.Fatal("disabled vibration returned a feedback channel")
	}
}

type feedbackTestSender struct{ added chan uint8 }

func (*feedbackTestSender) Init(uint8) error                      { return nil }
func (*feedbackTestSender) SendGamepads([]protocol.Gamepad) error { return nil }
func (s *feedbackTestSender) GamepadChanged(index uint8, added bool) error {
	if added {
		select {
		case s.added <- index:
		default:
		}
	}
	return nil
}

func TestFeedbackUsesStreamingSlotAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	close(ready)
	feedback := make(chan protocol.Vibration, 1)
	calls := make(chan motorCall, 3)
	pad := new(gamepad.Pad)
	sender := &feedbackTestSender{added: make(chan uint8, 1)}
	done := make(chan error, 1)
	go func() {
		done <- runWithFeedback(ctx, sender, testInputChannel{}, ready, &Keyboard{}, 0, 60,
			func() []*gamepad.Pad { return []*gamepad.Pad{pad} }, nil, nil, feedback,
			func(p *gamepad.Pad, left, right float64) { calls <- motorCall{p, left, right} })
	}()
	select {
	case <-sender.added:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("pad was not announced")
	}
	feedback <- protocol.Vibration{Index: 0, LeftMotor: 0.8, RightMotor: 0.3, Duration: time.Second}
	select {
	case call := <-calls:
		if call.pad != pad || call.left != 0.8 || call.right != 0.3 {
			t.Errorf("unexpected motor call: %+v", call)
		}
	case <-time.After(time.Second):
		cancel()
		t.Fatal("feedback was not applied")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case call := <-calls:
		if call.pad != pad || call.left != 0 || call.right != 0 {
			t.Errorf("motors not stopped: %+v", call)
		}
	default:
		t.Fatal("cancel did not stop motors")
	}
}
