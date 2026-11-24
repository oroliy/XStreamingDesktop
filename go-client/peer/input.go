package peer

import (
	"errors"
	"sync"
	"time"

	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"github.com/pion/webrtc/v4"
)

// InputSender serializes writes and assigns sequence numbers to controller reports.
type InputSender struct {
	mu          sync.Mutex
	channel     *webrtc.DataChannel
	started     time.Time
	seq         uint32
	initialized bool
}

func (p *Peer) Input() *InputSender {
	return &InputSender{channel: p.Channel("input"), started: time.Now()}
}
func (s *InputSender) Init(maxTouchpoints uint8) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initialized {
		return nil
	}
	if s.channel.ReadyState() != webrtc.DataChannelStateOpen {
		return errors.New("input channel is not open")
	}
	if err := s.channel.Send(protocol.ClientMetadata(s.seq, float64(time.Since(s.started).Microseconds())/1000, maxTouchpoints)); err != nil {
		return err
	}
	s.initialized = true
	return nil
}
func (s *InputSender) SendGamepads(frames []protocol.Gamepad) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		return errors.New("input channel is not initialized")
	}
	next := s.seq + 1
	packet, err := protocol.GamepadPacket(next, float64(time.Since(s.started).Microseconds())/1000, frames)
	if err != nil {
		return err
	}
	if err = s.channel.Send(packet); err != nil {
		return err
	}
	s.seq = next
	return nil
}
