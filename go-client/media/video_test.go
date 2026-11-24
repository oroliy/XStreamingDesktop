package media

import (
	"context"
	"image"
	"os/exec"
	"testing"
	"time"
)

func TestVideoDecoder(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	encoded, err := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=red:s=32x32:r=10:d=1", "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-f", "h264", "pipe:1").Output()
	if err != nil {
		t.Skipf("H.264 encoder unavailable: %v", err)
	}
	frames := make(chan image.Image, 20)
	decoder, err := NewVideoDecoder(ctx, func(frame image.Image) { frames <- frame })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = decoder.Write(encoded); err != nil {
		t.Fatal(err)
	}
	if err = decoder.Close(); err != nil {
		t.Fatal(err)
	}
	if len(frames) == 0 {
		t.Fatal("no decoded frames")
	}
	frame := <-frames
	if frame.Bounds().Dx() != 32 || frame.Bounds().Dy() != 32 {
		t.Fatal(frame.Bounds())
	}
}

// The decoder must emit frames while the input remains open for a live stream.
func TestVideoDecoderStreamsBeforeEOF(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	encoded, err := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=s=128x128:r=30:d=1", "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-g", "30", "-f", "h264", "pipe:1").Output()
	if err != nil {
		t.Skipf("H.264 encoder unavailable: %v", err)
	}
	frames := make(chan struct{}, 1)
	decoder, err := NewVideoDecoder(ctx, func(image.Image) {
		select {
		case frames <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := 0; i < 5; i++ {
			if _, err := decoder.Write(encoded); err != nil {
				return
			}
		}
	}()
	select {
	case <-frames:
	case <-ctx.Done():
		t.Error("decoder produced no frame before end of input")
	}
	cancel()
	<-writerDone
	_ = decoder.Close()
}
