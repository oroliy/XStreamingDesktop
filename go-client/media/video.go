// Package media contains stream media decoding helpers.
package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os/exec"
	"sync"

	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

// VideoDecoder decodes an Annex B H.264 stream with an installed ffmpeg.
// Frames are delivered serially on a decoder goroutine; callbacks must return promptly.
type VideoDecoder struct {
	cmd   *exec.Cmd
	input io.WriteCloser
	done  chan struct{}
	mu    sync.Mutex
	err   error
	once  sync.Once
}

func NewVideoDecoder(ctx context.Context, onFrame func(image.Image)) (*VideoDecoder, error) {
	if onFrame == nil {
		return nil, errors.New("frame callback required")
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-probesize", "32", "-analyzeduration", "0", "-flags", "low_delay", "-f", "h264", "-i", "pipe:0", "-an", "-f", "image2pipe", "-vcodec", "png", "pipe:1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	d := &VideoDecoder{cmd: cmd, input: stdin, done: make(chan struct{})}
	go func() {
		defer close(d.done)
		reader := bufio.NewReader(stdout)
		for {
			if _, e := reader.Peek(1); errors.Is(e, io.EOF) {
				break
			} else if e != nil {
				d.mu.Lock()
				d.err = e
				d.mu.Unlock()
				break
			}
			img, e := png.Decode(reader)
			if e != nil {
				if !errors.Is(e, io.EOF) {
					d.mu.Lock()
					d.err = fmt.Errorf("decode frame: %w", e)
					d.mu.Unlock()
				}
				break
			}
			onFrame(img)
		}
		err := cmd.Wait()
		d.mu.Lock()
		if d.err == nil && err != nil {
			d.err = fmt.Errorf("ffmpeg: %w", err)
		}
		d.mu.Unlock()
	}()
	return d, nil
}
func (d *VideoDecoder) Write(b []byte) (int, error) { return d.input.Write(b) }

// Close signals end of input and waits for all decoded frames.
func (d *VideoDecoder) Close() error {
	d.once.Do(func() { _ = d.input.Close(); <-d.done })
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

// ReceiveH264 reads RTP from a Pion track and writes depacketized H.264 samples.
// RTP loss and reordering are handled by Pion's bounded sample builder.
func ReceiveH264(track *webrtc.TrackRemote, out io.Writer) error {
	builder := samplebuilder.New(128, &codecs.H264Packet{}, 90000)
	for {
		packet, _, err := track.ReadRTP()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		builder.Push(packet)
		for {
			sample := builder.Pop()
			if sample == nil {
				break
			}
			if _, err = out.Write(sample.Data); err != nil {
				return err
			}
		}
	}
}
