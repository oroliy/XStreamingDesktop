// Package controller maps MyGo gamepad state to Xbox streaming input.
package controller

import (
	"context"
	"math"
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

// ApplyDeadzone removes small stick movements and rescales the remaining range.
func ApplyDeadzone(x, y, deadzone float64) (float64, float64) {
	if deadzone <= 0 {
		return x, y
	}
	magnitude := math.Hypot(x, y)
	if magnitude <= deadzone {
		return 0, 0
	}
	scale := math.Min((magnitude-deadzone)/(1-deadzone), 1) / magnitude
	return x * scale, y * scale
}

// Run sends up to four connected controllers until ctx is canceled.
// On disconnect it sends a neutral frame to release held buttons.
func Run(ctx context.Context, sender *peer.InputSender, channel *webrtc.DataChannel, ready <-chan struct{}, keyboard *Keyboard, deadzone float64, pollingRate int) error {
	select {
	case <-ctx.Done():
		return nil
	case <-ready:
	}
	if pollingRate != 60 && pollingRate != 125 && pollingRate != 250 {
		pollingRate = 60
	}
	ticker := time.NewTicker(time.Second / time.Duration(pollingRate))
	defer ticker.Stop()
	initialized := false
	slots := slotTracker{}
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
			added, removed := slots.update(pads, enabled)
			for _, index := range removed {
				if err := sender.SendGamepads([]protocol.Gamepad{{Index: index}}); err != nil {
					return err
				}
				if err := sender.GamepadChanged(index, false); err != nil {
					return err
				}
			}
			for _, index := range added {
				if err := sender.GamepadChanged(index, true); err != nil {
					return err
				}
			}
			frames := make([]protocol.Gamepad, 0, 4)
			for _, pad := range pads {
				index, ok := slots.pads[pad]
				if !ok {
					continue
				}
				state := Map(pad.State())
				state.LeftX, state.LeftY = ApplyDeadzone(state.LeftX, state.LeftY, deadzone)
				state.RightX, state.RightY = ApplyDeadzone(state.RightX, state.RightY, deadzone)
				state.Index = index
				if enabled && index == 0 {
					state = merge(state, virtual)
				}
				frames = append(frames, state)
			}
			if enabled {
				hasSlotZero := false
				for _, state := range frames {
					if state.Index == 0 {
						hasSlotZero = true
						break
					}
				}
				if !hasSlotZero {
					frames = append(frames, virtual)
				}
			}
			if len(frames) > 0 {
				if err := sender.SendGamepads(frames); err != nil {
					return err
				}
			}
		}
	}
}
