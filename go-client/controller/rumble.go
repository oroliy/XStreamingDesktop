package controller

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/game/gamepad"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
)

// xinputSlot recognizes the exact names used by MyGo's Windows XInput backend.
func xinputSlot(name string) (int, bool) {
	const prefix = "Xbox Controller "
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}
	number, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
	if err != nil || number < 1 || number > 4 || name != prefix+strconv.Itoa(number) {
		return 0, false
	}
	return number - 1, true
}

type rumbleEffect struct {
	pad   *gamepad.Pad
	timer *time.Timer
}

type rumbleManager struct {
	mu      sync.Mutex
	effects map[uint8]*rumbleEffect
	set     func(*gamepad.Pad, float64, float64)
	closed  bool
}

func newRumbleManager(set func(*gamepad.Pad, float64, float64)) *rumbleManager {
	return &rumbleManager{effects: make(map[uint8]*rumbleEffect), set: set}
}

// play replaces the previous effect in this streaming slot. Timers guard their
// identity so an old deadline cannot stop a newer effect.
func (m *rumbleManager) play(index uint8, pad *gamepad.Pad, v protocol.Vibration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.set == nil || pad == nil {
		return
	}
	m.stop(index)
	if v.Duration <= 0 || (v.LeftMotor == 0 && v.RightMotor == 0) {
		return
	}
	effect := &rumbleEffect{pad: pad}
	m.effects[index] = effect
	m.set(pad, v.LeftMotor, v.RightMotor)
	effect.timer = time.AfterFunc(v.Duration, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.effects[index] == effect {
			m.stop(index)
		}
	})
}

// stop must be called while m.mu is held.
func (m *rumbleManager) stop(index uint8) {
	effect := m.effects[index]
	if effect == nil {
		return
	}
	if effect.timer != nil {
		effect.timer.Stop()
	}
	delete(m.effects, index)
	m.set(effect.pad, 0, 0)
}

func (m *rumbleManager) reconcile(pads map[*gamepad.Pad]uint8) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index, effect := range m.effects {
		slot, present := pads[effect.pad]
		if !present || slot != index {
			m.stop(index)
		}
	}
}

func (m *rumbleManager) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for index := range m.effects {
		m.stop(index)
	}
}

func (m *rumbleManager) clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.effects {
		m.stop(index)
	}
}
