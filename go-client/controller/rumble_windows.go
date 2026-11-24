//go:build windows && (amd64 || arm64)

package controller

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo/plugins/game/gamepad"
)

var xinputVibration struct {
	once sync.Once
	proc uintptr
}

type xinputMotors struct{ left, right uint16 }

func setPadVibration(pad *gamepad.Pad, left, right float64) {
	// MyGo's Windows gamepad backend names its XInput slots this way.
	slot, ok := xinputSlot(pad.Name())
	if !ok {
		return
	}
	xinputVibration.once.Do(func() {
		for _, name := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
			lib, err := syscall.LoadLibrary(name)
			if err != nil {
				continue
			}
			proc, err := syscall.GetProcAddress(lib, "XInputSetState")
			if err == nil {
				xinputVibration.proc = proc
				return
			}
			_ = syscall.FreeLibrary(lib)
		}
	})
	if xinputVibration.proc == 0 {
		return
	}
	motors := xinputMotors{uint16(left * 65535), uint16(right * 65535)}
	_, _, _ = syscall.SyscallN(xinputVibration.proc, uintptr(slot), uintptr(unsafe.Pointer(&motors)))
}
