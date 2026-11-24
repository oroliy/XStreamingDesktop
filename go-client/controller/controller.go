// Package controller maps MyGo gamepad state to Xbox streaming input.
package controller

import (
	"context"
	"time"

	"github.com/egoist/mygo/plugins/game/gamepad"
	"github.com/oroliy/XStreamingDesktop/go-client/peer"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"github.com/pion/webrtc/v4"
)

func Map(s gamepad.State) protocol.Gamepad {
	g := protocol.Gamepad{}
	for _, b := range []struct {
		source gamepad.Button
		target protocol.Buttons
	}{{gamepad.South, protocol.A}, {gamepad.East, protocol.B}, {gamepad.West, protocol.X}, {gamepad.North, protocol.Y}, {gamepad.LeftShoulder, protocol.LeftShoulder}, {gamepad.RightShoulder, protocol.RightShoulder}, {gamepad.Back, protocol.View}, {gamepad.Start, protocol.Menu}, {gamepad.LeftStickButton, protocol.LeftThumb}, {gamepad.RightStickButton, protocol.RightThumb}, {gamepad.Up, protocol.DPadUp}, {gamepad.Down, protocol.DPadDown}, {gamepad.Left, protocol.DPadLeft}, {gamepad.Right, protocol.DPadRight}, {gamepad.Guide, protocol.Nexus}} {
		if s.Pressed(b.source) {
			g.Buttons |= b.target
		}
	}
	g.LeftX = float64(s.Axis(gamepad.LeftX))
	g.LeftY = float64(s.Axis(gamepad.LeftY))
	g.RightX = float64(s.Axis(gamepad.RightX))
	g.RightY = float64(s.Axis(gamepad.RightY))
	g.LeftTrigger = float64(s.Value(gamepad.LeftTrigger))
	g.RightTrigger = float64(s.Value(gamepad.RightTrigger))
	return g
}

// Run sends the first connected controller at 60Hz until ctx is canceled.
// On disconnect it sends a neutral frame to release held buttons.
func Run(ctx context.Context, sender *peer.InputSender, channel *webrtc.DataChannel, ready <-chan struct{}, keyboard *Keyboard) error {
	select {
	case <-ctx.Done():
		return nil
	case <-ready:
	}
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()
	initialized := false
	connected := false
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if channel.ReadyState() != webrtc.DataChannelStateOpen {
				continue
			}
			if !initialized {
				if err := sender.Init(0); err != nil {
					return err
				}
				initialized = true
			}
			pads := gamepad.Pads()
			enabled, virtual := keyboard.Snapshot()
			if len(pads) == 0 && !enabled {
				if connected {
					if err := sender.SendGamepads([]protocol.Gamepad{{}}); err != nil {
						return err
					}
					connected = false
				}
				continue
			}
			var state protocol.Gamepad
			if len(pads) > 0 {
				state = Map(pads[0].State())
			}
			if enabled {
				state = merge(state, virtual)
			}
			if err := sender.SendGamepads([]protocol.Gamepad{state}); err != nil {
				return err
			}
			connected = true
		}
	}
}
