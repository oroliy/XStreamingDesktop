package media

import (
	"context"
	"errors"
	"github.com/pion/rtp"
	"io"
	"os/exec"
	"reflect"
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
	p, err := NewAudioPlayer(ctx, 2, 100)
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
	if _, err := NewAudioPlayer(context.Background(), 3, 100); err == nil {
		t.Fatal("accepted invalid channel count")
	}
}

func TestAudioVolume(t *testing.T) {
	if _, err := NewAudioPlayer(context.Background(), 2, 101); err == nil {
		t.Fatal("accepted invalid volume")
	}
}

type collectedOpus struct{ sequences []uint16 }

func (w *collectedOpus) WriteRTP(packet *rtp.Packet) error {
	w.sequences = append(w.sequences, packet.SequenceNumber)
	return nil
}

func TestReceiveOpusReordersAndDropsDuplicates(t *testing.T) {
	for _, tc := range []struct {
		input []uint16
		want  []uint16
	}{
		{[]uint16{10, 12, 11, 12, 13}, []uint16{10, 11, 12, 13}},
		{[]uint16{65534, 0, 65535, 1}, []uint16{65534, 65535, 0, 1}},
		{[]uint16{10, 12, 13, 14}, []uint16{10, 12, 13, 14}},
		{[]uint16{10, 12}, []uint16{10, 12}},
		{[]uint16{10, 100}, []uint16{10, 100}},
	} {
		reader := &packetReader{err: io.EOF}
		for _, seq := range tc.input {
			reader.packets = append(reader.packets, &rtp.Packet{Header: rtp.Header{SequenceNumber: seq}})
		}
		var writer collectedOpus
		if err := receiveOpus(reader, &writer); err != nil || !reflect.DeepEqual(writer.sequences, tc.want) {
			t.Fatalf("input=%v got=%v want=%v error=%v", tc.input, writer.sequences, tc.want, err)
		}
	}
}

type failingOpus struct{ err error }

func (w failingOpus) WriteRTP(*rtp.Packet) error { return w.err }

func TestReceiveOpusPropagatesWriterError(t *testing.T) {
	failure := errors.New("audio sink failed")
	reader := &packetReader{packets: []*rtp.Packet{{Header: rtp.Header{SequenceNumber: 1}}}, err: io.EOF}
	if err := receiveOpus(reader, failingOpus{err: failure}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
