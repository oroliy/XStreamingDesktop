package protocol

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestGamepadPacket(t *testing.T) {
	b, err := GamepadPacket(0x12345678, 42.5, []Gamepad{{Index: 1, Buttons: A | DPadUp, LeftX: 1, LeftY: -1, RightX: -1, LeftTrigger: 0.5}})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 38 || binary.LittleEndian.Uint16(b) != 2 || binary.LittleEndian.Uint32(b[2:]) != 0x12345678 || math.Float64frombits(binary.LittleEndian.Uint64(b[6:])) != 42.5 || b[14] != 1 || b[15] != 1 {
		t.Fatal(b)
	}
	if binary.LittleEndian.Uint16(b[16:]) != uint16(A|DPadUp) || int16(binary.LittleEndian.Uint16(b[18:])) != 32767 || int16(binary.LittleEndian.Uint16(b[20:])) != 32767 || int16(binary.LittleEndian.Uint16(b[22:])) != -32767 || binary.LittleEndian.Uint16(b[26:]) != 32767 {
		t.Fatal(b)
	}
	if got := binary.LittleEndian.Uint32(b[30:]); got != 1|4096|1<<16|3<<18|3<<20 {
		t.Fatalf("physicality %x", got)
	}
}
func TestInputValidation(t *testing.T) {
	if _, err := GamepadPacket(0, 0, nil); err == nil {
		t.Fatal("accepted empty")
	}
	b := ClientMetadata(7, 10, 0)
	if len(b) != 15 || binary.LittleEndian.Uint16(b) != 8 || binary.LittleEndian.Uint32(b[2:]) != 7 {
		t.Fatal(b)
	}
}
