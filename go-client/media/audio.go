package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

// AudioPlayer streams Opus RTP through an Ogg container to ffplay.
// It requires ffplay and an available audio device.
type AudioPlayer struct {
	cmd        *exec.Cmd
	ogg        *oggwriter.OggWriter
	once       sync.Once
	err        error
	diagnostic *diagnosticTail
}

func NewAudioPlayer(ctx context.Context, channels uint16) (*AudioPlayer, error) {
	if channels != 1 && channels != 2 {
		return nil, errors.New("Opus requires one or two channels")
	}
	cmd := exec.CommandContext(ctx, "ffplay", "-loglevel", "error", "-nodisp", "-autoexit", "-probesize", "32", "-analyzeduration", "0", "-i", "pipe:0")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = io.Discard
	diagnostic := &diagnosticTail{}
	cmd.Stderr = diagnostic
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffplay: %w", err)
	}
	writer, err := oggwriter.NewWith(stdin, 48000, channels)
	if err != nil {
		_ = stdin.Close()
		if waitErr := cmd.Wait(); waitErr != nil {
			return nil, processError("ffplay", waitErr, diagnostic)
		}
		return nil, err
	}
	return &AudioPlayer{cmd: cmd, ogg: writer, diagnostic: diagnostic}, nil
}
func (p *AudioPlayer) WriteRTP(packet *rtp.Packet) error { return p.ogg.WriteRTP(packet) }
func (p *AudioPlayer) Close() error {
	p.once.Do(func() {
		err := p.ogg.Close()
		waitErr := p.cmd.Wait()
		if waitErr != nil {
			p.err = processError("ffplay", waitErr, p.diagnostic)
		} else if err != nil {
			p.err = err
		}
	})
	return p.err
}

// ReceiveOpus forwards a remote Opus track to the audio player.
func ReceiveOpus(track *webrtc.TrackRemote, p *AudioPlayer) error {
	for {
		packet, _, err := track.ReadRTP()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err = p.WriteRTP(packet); err != nil {
			return err
		}
	}
}
