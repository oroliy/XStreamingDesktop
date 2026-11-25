package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/game/gamepad"
	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/controller"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
)

type controllerTestPad struct {
	name  string
	state protocol.Gamepad
}

func readControllerTestPads() []controllerTestPad {
	pads := gamepad.Pads()
	result := make([]controllerTestPad, 0, min(len(pads), 4))
	for _, pad := range pads {
		if len(result) == 4 {
			break
		}
		state := pad.State()
		if pad.Connected() {
			result = append(result, controllerTestPad{name: pad.Name(), state: controller.Map(state)})
		}
	}
	return result
}

func controllerButtonNames(buttons protocol.Buttons) string {
	var names []string
	for _, item := range []struct {
		mask protocol.Buttons
		name string
	}{
		{protocol.A, "A"}, {protocol.B, "B"}, {protocol.X, "X"}, {protocol.Y, "Y"},
		{protocol.DPadUp, "↑"}, {protocol.DPadDown, "↓"}, {protocol.DPadLeft, "←"}, {protocol.DPadRight, "→"},
		{protocol.LeftShoulder, "LB"}, {protocol.RightShoulder, "RB"}, {protocol.LeftThumb, "LS"}, {protocol.RightThumb, "RS"},
		{protocol.Menu, "Menu"}, {protocol.View, "View"}, {protocol.Nexus, "Xbox"},
	} {
		if buttons&item.mask != 0 {
			names = append(names, item.name)
		}
	}
	return strings.Join(names, ", ")
}

func pollControllerTest(ctx context.Context, sample func() []controllerTestPad, publish func([]controllerTestPad), interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		pads := sample()
		if ctx.Err() != nil {
			return
		}
		publish(pads)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// These methods run on the UI thread; sampled state is handed back through Update.
func (a *app) startControllerTest() {
	if a.controllerTestCancel != nil || a.ctx == nil || a.win == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.controllerTestCancel = cancel
	a.controllerTestGeneration++
	generation := a.controllerTestGeneration
	var previous []controllerTestPad
	go pollControllerTest(ctx, readControllerTestPads, func(pads []controllerTestPad) {
		if slices.Equal(previous, pads) {
			return
		}
		previous = pads
		a.update(func() {
			if ctx.Err() == nil && a.controllerTestCancel != nil && a.controllerTestGeneration == generation && a.streamCancel == nil {
				a.controllerTestPads = pads
			}
		})
	}, 100*time.Millisecond)
}
func (a *app) stopControllerTest() {
	if a.controllerTestCancel != nil {
		a.controllerTestCancel()
		a.controllerTestCancel = nil
	}
	a.controllerTestGeneration++
	a.controllerTestPads = nil
}
func (a *app) controllerTestView(c *ui.Context) {
	label := "Test controllers"
	if a.controllerTestCancel != nil {
		label = "Stop controller test"
	}
	if ui.Button(c.Key("controller-test"), a.tr(label)).Clicked() {
		if a.controllerTestCancel == nil {
			a.startControllerTest()
		} else {
			a.stopControllerTest()
		}
	}
	if a.controllerTestCancel == nil {
		return
	}
	if len(a.controllerTestPads) == 0 {
		ui.Text(c, a.tr("No controllers detected."))
		return
	}
	for i, pad := range a.controllerTestPads {
		ui.Text(c, fmt.Sprintf("%d. %s", i+1, pad.name)).Bold()
		names := controllerButtonNames(pad.state.Buttons)
		if names == "" {
			ui.Text(c, a.tr("Buttons: none"))
		} else {
			ui.Text(c, a.tr("Buttons: ")+names)
		}
		ui.Text(c, fmt.Sprintf("L: %.2f, %.2f  R: %.2f, %.2f", pad.state.LeftX, pad.state.LeftY, pad.state.RightX, pad.state.RightY))
		ui.Text(c, fmt.Sprintf("LT: %.2f  RT: %.2f", pad.state.LeftTrigger, pad.state.RightTrigger))
	}
}
