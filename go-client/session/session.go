// Package session connects the streaming service signaling to a WebRTC peer.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/oroliy/XStreamingDesktop/go-client/peer"
	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
	"github.com/pion/webrtc/v4"
)

type Session struct {
	client   *streaming.Client
	peer     *peer.Peer
	id       string
	cancel   context.CancelFunc
	once     sync.Once
	closeErr error
}
type Options struct {
	Play          streaming.PlayOptions
	TransferToken func(context.Context) (string, error)
	RTC           webrtc.Configuration
	OnTrack       func(*webrtc.TrackRemote, *webrtc.RTPReceiver)
	OnError       func(error)
	PreferIPv6    bool
}

// Start creates and provisions a session, negotiates SDP and exchanges ICE.
// The caller must call Close even after the supplied context is cancelled.
func Start(ctx context.Context, client *streaming.Client, opts Options) (_ *Session, err error) {
	result, err := client.Start(ctx, opts.Play)
	if err != nil {
		return nil, err
	}
	id, err := client.SessionID(result)
	if err != nil {
		return nil, err
	}
	live, cancel := context.WithCancel(ctx)
	s := &Session{client: client, id: id, cancel: cancel}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()
	if err = client.WaitProvisioned(live, id, opts.TransferToken); err != nil {
		return nil, err
	}
	s.peer, err = peer.New(opts.RTC, opts.OnTrack)
	if err != nil {
		return nil, err
	}
	s.peer.StartControl(live, opts.OnError)
	offer, err := s.peer.Offer(live)
	if err != nil {
		return nil, err
	}
	raw, err := client.ExchangeSDP(live, id, offer.SDP, false)
	if err != nil {
		return nil, err
	}
	var answer struct {
		SDP string `json:"sdp"`
	}
	if err = json.Unmarshal(raw, &answer); err != nil {
		return nil, err
	}
	if err = s.peer.SetAnswer(answer.SDP); err != nil {
		return nil, err
	}
	local, err := peer.LocalCandidates(offer.SDP)
	if err != nil {
		return nil, err
	}
	raw, err = client.ExchangeICE(live, id, local)
	if err != nil {
		return nil, err
	}
	var remote []streaming.ICECandidate
	if err = json.Unmarshal(raw, &remote); err != nil {
		return nil, err
	}
	remote, err = streaming.NormalizeICE(remote, opts.PreferIPv6)
	if err != nil {
		return nil, err
	}
	for _, candidate := range remote {
		line, e := strconv.ParseUint(candidate.SDPMLineIndex, 10, 16)
		if e != nil {
			return nil, fmt.Errorf("invalid ICE m-line: %w", e)
		}
		if err = s.peer.AddCandidate(candidate.Candidate, candidate.SDPMid, uint16(line)); err != nil {
			return nil, err
		}
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-live.Done():
				return
			case <-ticker.C:
				if e := client.Keepalive(live, id); e != nil && opts.OnError != nil {
					opts.OnError(e)
				}
			}
		}
	}()
	return s, nil
}
func (s *Session) Ready() <-chan struct{}            { return s.peer.Ready() }
func (s *Session) InputChannel() *webrtc.DataChannel { return s.peer.Channel("input") }
func (s *Session) Input() *peer.InputSender          { return s.peer.Input() }
func (s *Session) Close() error {
	s.once.Do(func() {
		s.cancel()
		if s.peer != nil {
			_ = s.peer.Close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.closeErr = s.client.Stop(ctx, s.id)
	})
	return s.closeErr
}
