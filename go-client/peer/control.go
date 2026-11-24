package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"github.com/pion/webrtc/v4"
)

// StartControl installs the initial message handshake and control messages.
// Call before negotiating. The caller cancels ctx before closing the peer.
// Other message targets and controller input are not handled yet.
func (p *Peer) StartControl(ctx context.Context, onError func(error)) {
	message, control := p.Channel("message"), p.Channel("control")
	state := protocol.NewMessageChannel()
	var mu sync.Mutex
	ready, controlOpen, started := false, false, false
	report := func(err error) {
		if err != nil && onError != nil {
			onError(err)
		}
	}
	send := func(dc *webrtc.DataChannel, v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return dc.SendText(string(b))
	}
	start := func() {
		mu.Lock()
		if !ready || !controlOpen || started {
			mu.Unlock()
			return
		}
		started = true
		mu.Unlock()
		if err := send(control, protocol.Authorization()); err != nil {
			report(err)
			return
		}
		if err := send(control, protocol.GamepadChanged(0, false)); err != nil {
			report(err)
			return
		}
		close(p.ready)
		go func() {
			timer := time.NewTimer(500 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			if err := send(control, protocol.GamepadChanged(0, true)); err != nil {
				report(err)
				return
			}
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := send(control, protocol.KeyframeRequest()); err != nil {
						report(err)
						return
					}
				}
			}
		}()
	}
	control.OnOpen(func() { mu.Lock(); controlOpen = true; mu.Unlock(); start() })
	message.OnOpen(func() { report(send(message, state.Handshake())) })
	message.OnMessage(func(msg webrtc.DataChannelMessage) {
		data := bytes.ReplaceAll(msg.Data, []byte{0}, nil)
		var envelope protocol.Envelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			report(fmt.Errorf("message channel: %w", err))
			return
		}
		if envelope.Type != "HandshakeAck" {
			if reply := state.Incoming(envelope); reply != nil {
				report(send(message, reply))
			}
			return
		}
		config, err := state.HandleHandshake(data)
		if err != nil {
			report(err)
			return
		}
		for _, v := range config {
			if err = send(message, v); err != nil {
				report(err)
				return
			}
		}
		mu.Lock()
		ready = true
		mu.Unlock()
		start()
	})
}
