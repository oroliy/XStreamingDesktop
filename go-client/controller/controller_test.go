package controller

import (
	"github.com/egoist/mygo/plugins/game/gamepad"
	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"math"
	"testing"
)

func TestMap(t *testing.T) {
	var s gamepad.State
	s.Buttons[gamepad.South] = 1
	s.Buttons[gamepad.Up] = 1
	s.Buttons[gamepad.LeftTrigger] = 0.5
	s.Axes[gamepad.LeftY] = -0.75
	g := Map(s)
	if g.Buttons != protocol.A|protocol.DPadUp || g.LeftTrigger != 0.5 || g.LeftY != -0.75 {
		t.Fatalf("%+v", g)
	}
}

func TestKeyboardMappingAndMerge(t *testing.T) {
	pressed := map[ui.Key]bool{ui.KeyW: true, ui.KeyK: true, ui.Key4: true, ui.KeyRight: true}
	v := MapKeys(func(k ui.Key) bool { return pressed[k] })
	if v.LeftY != -1 || v.Buttons != protocol.A|protocol.DPadRight || v.RightTrigger != 1 {
		t.Fatalf("%+v", v)
	}
	base := protocol.Gamepad{Buttons: protocol.B, LeftX: 0.4}
	out := merge(base, v)
	if out.Buttons != protocol.A|protocol.B|protocol.DPadRight || out.LeftX != 0.4 || out.LeftY != -1 {
		t.Fatalf("%+v", out)
	}
	var k Keyboard
	k.Set(true, v)
	enabled, _ := k.Snapshot()
	if !enabled {
		t.Fatal("not enabled")
	}
	k.Set(false, v)
	enabled, v = k.Snapshot()
	if enabled || v.Buttons != 0 {
		t.Fatal("keyboard not cleared")
	}
}

func TestStickDeadzone(t *testing.T) {
	if x, y := ApplyDeadzone(0.03, 0.04, 0.1); x != 0 || y != 0 {
		t.Fatalf("center drift: %v %v", x, y)
	}
	if x, y := ApplyDeadzone(0.55, 0, 0.1); math.Abs(x-0.5) > 1e-9 || y != 0 {
		t.Fatalf("scaled stick: %v %v", x, y)
	}
	if x, y := ApplyDeadzone(0.03, 0.04, 0); x != 0.03 || y != 0.04 {
		t.Fatalf("disabled deadzone: %v %v", x, y)
	}
}
