package controller

import (
	"github.com/egoist/mygo/plugins/game/gamepad"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
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
