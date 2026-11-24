package media

import (
	"context"
	"github.com/pion/rtp"
	"os/exec"
	"testing"
	"time"
)

func TestAudioPlayer(t *testing.T) {
	if _, err := exec.LookPath("ffplay"); err != nil {
		t.Skip("ffplay unavailable")
	}
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := NewAudioPlayer(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		err = p.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: uint16(i), Timestamp: uint32(i * 960)}, Payload: []byte{0xf8, 0xff, 0xfe}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = p.Close(); err != nil {
		t.Fatal(err)
	}
	if err = p.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestAudioChannels(t *testing.T) {
	if _, err := NewAudioPlayer(context.Background(), 3); err == nil {
		t.Fatal("accepted invalid channel count")
	}
}
