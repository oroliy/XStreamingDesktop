package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oroliy/XStreamingDesktop/go-client/media"
	"github.com/oroliy/XStreamingDesktop/go-client/session"
	"github.com/oroliy/XStreamingDesktop/go-client/settings"
	pionmedia "github.com/pion/webrtc/v4/pkg/media"
)

type microphoneWriter interface {
	WriteSample(pionmedia.Sample) error
}

type microphoneSession interface {
	StartMicrophone(context.Context) (microphoneWriter, error)
	StopMicrophone() error
}

type liveMicrophoneSession struct{ *session.Session }

func (s liveMicrophoneSession) StartMicrophone(ctx context.Context) (microphoneWriter, error) {
	return s.Session.StartMicrophone(ctx)
}

type microphoneEncoder func(context.Context, []string, func(pionmedia.Sample) error) error

func microphoneSourceOptions(sources []string, selected string) []string {
	options := []string{"default"}
	seen := map[string]bool{"default": true}
	for _, source := range append(append([]string(nil), sources...), selected) {
		if source != "" && !seen[source] {
			seen[source] = true
			options = append(options, source)
		}
	}
	return options
}

func (a *app) refreshMicrophoneSources() {
	if a.micSourcesLoading || a.ctx == nil || a.ctx.Err() != nil {
		return
	}
	a.micSourcesLoading = true
	a.micSourcesStatus = "Loading microphone inputs…"
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
		defer cancel()
		sources, err := media.PulseSources(ctx)
		a.update(func() {
			if a.ctx.Err() != nil {
				return
			}
			a.finishMicrophoneSources(sources, err)
		})
	}()
}

func (a *app) finishMicrophoneSources(sources []string, err error) {
	a.micSourcesLoading = false
	if err != nil {
		a.micSourcesStatus = "Unable to list microphone inputs: " + err.Error()
		return
	}
	a.micSources = sources
	a.micSourcesStatus = "Microphone inputs refreshed."
	if len(sources) == 0 {
		a.micSourcesStatus = "No microphone inputs found."
	}
}

// runMicrophone negotiates first; cancellation during negotiation prevents capture
// without interrupting signaling for the enclosing stream.
func runMicrophone(streamCtx, micCtx context.Context, source string, s microphoneSession, encode microphoneEncoder, active func()) (error, error) {
	track, err := s.StartMicrophone(streamCtx)
	if err != nil {
		return err, nil
	}
	if err := micCtx.Err(); err != nil {
		return err, s.StopMicrophone()
	}
	announced := false
	err = encode(micCtx, []string{"-f", "pulse", "-i", source}, func(sample pionmedia.Sample) error {
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

func (a *app) saveMicrophoneSource() {
	source := strings.TrimSpace(a.micSourceDraft)
	if source == "" {
		source = "default"
	}
	next := a.prefs
	next.MicrophoneSource = source
	if err := next.Validate(); err != nil {
		a.status = "Invalid microphone input name."
		return
	}
	if a.settingsPath != "" {
		if err := settings.Save(a.settingsPath, next); err != nil {
			a.status = "Unable to save settings: " + err.Error()
			return
		}
	}
	a.prefs = next
	a.micSourceDraft = source
	a.status = "Microphone input saved."
}

// startMicrophone and clearMicrophone run on the UI thread.
func (a *app) startMicrophone() {
	if a.micSession == nil || a.streamCtx == nil || a.micCancel != nil {
		return
	}
	streamCtx, s, source := a.streamCtx, a.micSession, a.prefs.MicrophoneSource
	if source == "" {
		source = "default"
	}
	micCtx, cancel := context.WithCancel(streamCtx)
	a.micCtx, a.micCancel = micCtx, cancel
	a.micStatus = "Starting microphone…"
	if a.streamWG != nil {
		a.streamWG.Add(1)
	}
	go a.captureMicrophone(streamCtx, micCtx, cancel, s, source)
}

func (a *app) captureMicrophone(streamCtx, micCtx context.Context, cancel context.CancelFunc, s *session.Session, source string) {
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
	err, stopErr := runMicrophone(streamCtx, micCtx, source, liveMicrophoneSession{s}, media.EncodeMicrophone, func() {
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
