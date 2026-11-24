package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/oroliy/XStreamingDesktop/go-client/media"
	"github.com/oroliy/XStreamingDesktop/go-client/session"
	"github.com/pion/webrtc/v4"
	pionmedia "github.com/pion/webrtc/v4/pkg/media"
)

type microphoneSession interface {
	StartMicrophone(context.Context) (*webrtc.TrackLocalStaticSample, error)
	StopMicrophone() error
}

type microphoneEncoder func(context.Context, []string, func(pionmedia.Sample) error) error

// runMicrophone negotiates first; cancellation during negotiation prevents capture
// without interrupting signaling for the enclosing stream.
func runMicrophone(streamCtx, micCtx context.Context, s microphoneSession, encode microphoneEncoder, active func()) (error, error) {
	track, err := s.StartMicrophone(streamCtx)
	if err != nil {
		return err, nil
	}
	if err := micCtx.Err(); err != nil {
		return err, s.StopMicrophone()
	}
	announced := false
	err = encode(micCtx, []string{"-f", "pulse", "-i", "default"}, func(sample pionmedia.Sample) error {
		if err := micCtx.Err(); err != nil {
			return err
		}
		if err := track.WriteSample(sample); err != nil {
			return err
		}
		if !announced {
			announced = true
			active()
		}
		return nil
	})
	if err == nil && micCtx.Err() == nil {
		err = errors.New("audio capture ended")
	}
	return err, s.StopMicrophone()
}

// startMicrophone and clearMicrophone run on the UI thread.
func (a *app) startMicrophone() {
	if a.micSession == nil || a.streamCtx == nil || a.micCancel != nil {
		return
	}
	streamCtx, s := a.streamCtx, a.micSession
	micCtx, cancel := context.WithCancel(streamCtx)
	a.micCtx, a.micCancel = micCtx, cancel
	a.micStatus = "Starting microphone…"
	if a.streamWG != nil {
		a.streamWG.Add(1)
	}
	go a.captureMicrophone(streamCtx, micCtx, cancel, s)
}

func (a *app) captureMicrophone(streamCtx, micCtx context.Context, cancel context.CancelFunc, s *session.Session) {
	if a.streamWG != nil {
		defer a.streamWG.Done()
	}
	defer cancel()
	finished := make(chan struct{})
	go func() {
		select {
		case <-s.Done():
			cancel()
		case <-finished:
		}
	}()
	defer close(finished)
	err, stopErr := runMicrophone(streamCtx, micCtx, s, media.EncodeMicrophone, func() {
		a.update(func() {
			if a.micCtx == micCtx && micCtx.Err() == nil {
				a.micStatus = "Microphone on."
			}
		})
	})
	wasCanceled := micCtx.Err() != nil
	a.update(func() {
		if a.micCtx != micCtx {
			return
		}
		a.micCtx, a.micCancel = nil, nil
		a.micStopping = false
		a.micStatus = ""
		if stopErr != nil {
			a.micStatus = fmt.Sprintf("Microphone failed: %v", stopErr)
		} else if err != nil && !wasCanceled {
			a.micStatus = fmt.Sprintf("Microphone failed: %v", err)
		}
	})
}

func (a *app) clearMicrophone() {
	if a.micCancel != nil {
		a.micCancel()
	}
	a.micSession, a.micCtx, a.micCancel = nil, nil, nil
	a.micStopping = false
	a.micStatus = ""
}
