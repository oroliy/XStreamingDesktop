package peer

import (
	"context"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

func TestLocalNegotiationAndDataChannel(t *testing.T) {
	local, err := New(webrtc.Configuration{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	remote, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	received := make(chan string, 1)
	remote.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() == "input" {
			dc.OnMessage(func(msg webrtc.DataChannelMessage) { received <- string(msg.Data) })
		}
	})
	opened := make(chan struct{})
	local.Channel("input").OnOpen(func() { close(opened) })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	offer, err := local.Offer(ctx)
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
	gathered := webrtc.GatheringCompletePromise(remote)
	if err = remote.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gathered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err = local.SetAnswer(remote.LocalDescription().SDP); err != nil {
		t.Fatal(err)
	}
	select {
	case <-opened:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err = local.Channel("input").SendText("hello"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-received:
		if msg != "hello" {
			t.Fatal(msg)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
func TestRejectEmptyAnswer(t *testing.T) {
	p, err := New(webrtc.Configuration{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if p.SetAnswer("") == nil {
		t.Fatal("accepted empty answer")
	}
}

func TestReceiveVideoRTP(t *testing.T) {
	packets := make(chan []byte, 1)
	local, err := New(webrtc.Configuration{}, func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		packet, _, err := track.ReadRTP()
		if err == nil {
			packets <- packet.Payload
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	remote, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "stream")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = remote.AddTrack(track); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	offer, err := local.Offer(ctx)
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
	if err = local.SetAnswer(remote.LocalDescription().SDP); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = track.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: 1, Timestamp: 9000}, Payload: []byte{0x65, 0x01}})
		case data := <-packets:
			if len(data) != 2 || data[0] != 0x65 {
				t.Fatal(data)
			}
			return
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
