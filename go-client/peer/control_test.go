package peer

import (
	"context"
	"encoding/json"
	"github.com/pion/webrtc/v4"
	"testing"
	"time"
)

func TestControlHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := New(webrtc.Configuration{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	errors := make(chan error, 4)
	p.StartControl(ctx, func(err error) { errors <- err })
	remote, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	got := make(chan string, 16)
	replies := make(chan string, 1)
	remote.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			var body map[string]any
			if json.Unmarshal(msg.Data, &body) != nil {
				return
			}
			if dc.Label() == "message" && body["type"] == "Handshake" {
				_ = dc.SendText(`{"type":"HandshakeAck","version":"messageV1"}`)
				_ = dc.SendText(`{"type":"TransactionStart","id":"t1","target":"/unsupported","cv":"server.2"}`)
			}
			if dc.Label() == "message" && body["type"] == "Unhandled" {
				replies <- body["cv"].(string)
			}
			if dc.Label() == "control" {
				if name, ok := body["message"].(string); ok {
					got <- name
				}
			}
		})
	})
	offer, err := p.Offer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = remote.SetRemoteDescription(offer); err != nil {
		t.Fatal(err)
	}
	answer, err := remote.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	done := webrtc.GatheringCompletePromise(remote)
	if err = remote.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err = p.SetAnswer(remote.LocalDescription().SDP); err != nil {
		t.Fatal(err)
	}
	select {
	case cv := <-replies:
		if cv != "server.3" {
			t.Fatal(cv)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for _, expected := range []string{"authorizationRequest", "gamepadChanged"} {
		select {
		case msg := <-got:
			if msg != expected {
				t.Fatalf("got %q want %q", msg, expected)
			}
		case err := <-errors:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
