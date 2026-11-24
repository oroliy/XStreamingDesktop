package session

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
	"github.com/pion/webrtc/v4"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionNegotiationAndCleanup(t *testing.T) {
	remote, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	inputPackets := make(chan []byte, 4)
	controlMessages := make(chan string, 4)
	remote.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			switch dc.Label() {
			case "message":
				var body struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(msg.Data, &body) == nil && body.Type == "Handshake" {
					if err := dc.SendText(`{"type":"HandshakeAck","version":"messageV1"}`); err != nil {
						t.Error(err)
					}
				}
			case "control":
				var body struct {
					Message string `json:"message"`
				}
				if json.Unmarshal(msg.Data, &body) == nil {
					select {
					case controlMessages <- body.Message:
					default:
					}
				}
			case "input":
				select {
				case inputPackets <- append([]byte(nil), msg.Data...):
				default:
				}
			}
		})
	})
	var answer string
	var stopped atomic.Int32
	stoppedSignal := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v5/sessions/home/play":
			fmt.Fprint(w, `{"sessionPath":"/v5/sessions/home/abc"}`)
		case r.URL.Path == "/v5/sessions/home/abc/state":
			fmt.Fprint(w, `{"state":"Provisioned"}`)
		case r.URL.Path == "/v5/sessions/home/abc/sdp" && r.Method == "POST":
			var offer struct {
				SDP string `json:"sdp"`
			}
			if e := json.NewDecoder(r.Body).Decode(&offer); e != nil {
				t.Error(e)
				return
			}
			if e := remote.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.SDP}); e != nil {
				t.Error(e)
				return
			}
			a, e := remote.CreateAnswer(nil)
			if e != nil {
				t.Error(e)
				return
			}
			done := webrtc.GatheringCompletePromise(remote)
			if e = remote.SetLocalDescription(a); e != nil {
				t.Error(e)
				return
			}
			<-done
			answer = remote.LocalDescription().SDP
			w.WriteHeader(204)
		case r.URL.Path == "/v5/sessions/home/abc/sdp":
			b, _ := json.Marshal(map[string]string{"sdp": answer})
			json.NewEncoder(w).Encode(map[string]string{"exchangeResponse": string(b)})
		case r.URL.Path == "/v5/sessions/home/abc/ice" && r.Method == "POST":
			w.WriteHeader(204)
		case r.URL.Path == "/v5/sessions/home/abc/ice":
			fmt.Fprint(w, `{"exchangeResponse":"[]"}`)
		case r.URL.Path == "/v5/sessions/home/abc" && r.Method == "DELETE":
			stopped.Add(1)
			select {
			case stoppedSignal <- struct{}{}:
			default:
			}
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := streaming.NewClient(server.URL, "token", "home", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connected := make(chan struct{}, 1)
	s, err := Start(ctx, client, Options{Play: streaming.PlayOptions{Target: "console"}, OnConnectionState: func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			select {
			case connected <- struct{}{}:
			default:
			}
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-connected:
	case <-ctx.Done():
		t.Fatal("connection state did not reach connected")
	}
	select {
	case <-s.Ready():
	case <-ctx.Done():
		t.Fatal("control handshake did not complete")
	}
	for _, want := range []string{"authorizationRequest", "gamepadChanged"} {
		select {
		case got := <-controlMessages:
			if got != want {
				t.Fatalf("control message %q, want %q", got, want)
			}
		case <-ctx.Done():
			t.Fatal("missing control message")
		}
	}
	sender := s.Input()
	for s.InputChannel().ReadyState() != webrtc.DataChannelStateOpen {
		select {
		case <-ctx.Done():
			t.Fatal("input channel did not open")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := sender.Init(0); err != nil {
		t.Fatal(err)
	}
	if err := sender.SendGamepads([]protocol.Gamepad{{Buttons: protocol.A, LeftX: 1}}); err != nil {
		t.Fatal(err)
	}
	for _, reportType := range []uint16{8, 2} {
		select {
		case packet := <-inputPackets:
			if len(packet) < 15 || binary.LittleEndian.Uint16(packet) != reportType {
				t.Fatalf("unexpected input report %x", packet)
			}
			if reportType == 2 && (len(packet) != 38 || binary.LittleEndian.Uint16(packet[16:]) != uint16(protocol.A)) {
				t.Fatalf("invalid gamepad report %x", packet)
			}
		case <-ctx.Done():
			t.Fatal("missing input report")
		}
	}
	cancel()
	select {
	case <-stoppedSignal:
	case <-time.After(3 * time.Second):
		t.Fatal("session not stopped after context cancellation")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if stopped.Load() != 1 {
		t.Fatal("session not stopped exactly once")
	}
}

// A provisioning failure must release the remote session even if the caller's context is canceled.
func TestFailedProvisioningCleansUp(t *testing.T) {
	var stopped atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v5/sessions/home/play":
			fmt.Fprint(w, `{"sessionPath":"/v5/sessions/failed"}`)
		case r.URL.Path == "/v5/sessions/home/failed/state":
			fmt.Fprint(w, `{"state":"Failed","errorDetails":{"code":"no-capacity"}}`)
		case r.URL.Path == "/v5/sessions/home/failed" && r.Method == http.MethodDelete:
			stopped.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := streaming.NewClient(server.URL, "token", "home", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	s, err := Start(context.Background(), client, Options{Play: streaming.PlayOptions{Target: "console"}})
	if err == nil || s != nil {
		t.Fatalf("unexpected session or error: %v", err)
	}
	if stopped.Load() != 1 {
		t.Fatalf("stopped %d times", stopped.Load())
	}
}
func TestCanceledProvisioningCleansUp(t *testing.T) {
	var stopped atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v5/sessions/home/play":
			fmt.Fprint(w, `{"sessionPath":"/v5/sessions/canceled"}`)
		case r.URL.Path == "/v5/sessions/home/canceled/state":
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/v5/sessions/home/canceled" && r.Method == http.MethodDelete:
			stopped.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := streaming.NewClient(server.URL, "token", "home", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	s, err := Start(ctx, client, Options{Play: streaming.PlayOptions{Target: "console"}})
	if s != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected result: %v", err)
	}
	if stopped.Load() != 1 {
		t.Fatalf("stopped %d times", stopped.Load())
	}
}

func TestSignalingFailureCleansUp(t *testing.T) {
	var stopped atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v5/sessions/home/play":
			fmt.Fprint(w, `{"sessionPath":"/v5/sessions/signaling"}`)
		case r.URL.Path == "/v5/sessions/home/signaling/state":
			fmt.Fprint(w, `{"state":"Provisioned"}`)
		case r.URL.Path == "/v5/sessions/home/signaling/sdp" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusBadGateway)
		case r.URL.Path == "/v5/sessions/home/signaling" && r.Method == http.MethodDelete:
			stopped.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := streaming.NewClient(server.URL, "token", "home", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := Start(ctx, client, Options{Play: streaming.PlayOptions{Target: "console"}})
	var response *streaming.HTTPError
	if s != nil || !errors.As(err, &response) || response.Status != http.StatusBadGateway {
		t.Fatalf("unexpected result: %v", err)
	}
	if stopped.Load() != 1 {
		t.Fatalf("stopped %d times", stopped.Load())
	}
}

func TestStartupErrorPreservesCleanupFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v5/sessions/home/play":
			fmt.Fprint(w, `{"sessionPath":"/v5/sessions/failed"}`)
		case r.URL.Path == "/v5/sessions/home/failed/state":
			fmt.Fprint(w, `{"state":"Failed","errorDetails":{"code":"no-capacity"}}`)
		case r.URL.Path == "/v5/sessions/home/failed" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusBadGateway)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := streaming.NewClient(server.URL, "token", "home", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = Start(context.Background(), client, Options{Play: streaming.PlayOptions{Target: "console"}})
	var cleanup *CleanupError
	var httpErr *streaming.HTTPError
	if err == nil || !strings.Contains(err.Error(), "no-capacity") || !errors.As(err, &cleanup) || !errors.As(cleanup, &httpErr) || httpErr.Status != http.StatusBadGateway {
		t.Fatalf("expected provisioning and cleanup errors, got %v", err)
	}
}

func TestConnectionTimeoutAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go monitorConnection(ctx, make(chan struct{}), 10*time.Millisecond, cancel)
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), ErrConnectionTimeout) {
			t.Fatal(context.Cause(ctx))
		}
	case <-time.After(time.Second):
		t.Fatal("connection did not time out")
	}
}

func TestConnectedSessionIsNotTimedOut(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	connected := make(chan struct{})
	close(connected)
	done := make(chan struct{})
	go func() { monitorConnection(ctx, connected, time.Millisecond, cancel); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor did not stop after connection")
	}
	if err := context.Cause(ctx); err != nil {
		t.Fatal(err)
	}
}

// No ICE candidates are exchanged, so signaling succeeds but the connection stalls.
func TestConnectionTimeoutStopsRemoteSession(t *testing.T) {
	remote, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	withoutCandidates := func(sdp string) string {
		lines := strings.Split(sdp, "\r\n")
		filtered := lines[:0]
		for _, line := range lines {
			if !strings.HasPrefix(line, "a=candidate:") && line != "a=end-of-candidates" {
				filtered = append(filtered, line)
			}
		}
		return strings.Join(filtered, "\r\n")
	}
	var answer string
	var stops atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v5/sessions/home/play":
			fmt.Fprint(w, `{"sessionPath":"/v5/sessions/timeout"}`)
		case r.URL.Path == "/v5/sessions/home/timeout/state":
			fmt.Fprint(w, `{"state":"Provisioned"}`)
		case r.URL.Path == "/v5/sessions/home/timeout/sdp" && r.Method == http.MethodPost:
			var offer struct {
				SDP string `json:"sdp"`
			}
			if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if err := remote.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: withoutCandidates(offer.SDP)}); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			desc, err := remote.CreateAnswer(nil)
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			gathered := webrtc.GatheringCompletePromise(remote)
			if err := remote.SetLocalDescription(desc); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			<-gathered
			answer = withoutCandidates(remote.LocalDescription().SDP)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/v5/sessions/home/timeout/sdp":
			payload, _ := json.Marshal(map[string]string{"sdp": answer})
			_ = json.NewEncoder(w).Encode(map[string]string{"exchangeResponse": string(payload)})
		case r.URL.Path == "/v5/sessions/home/timeout/ice" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/v5/sessions/home/timeout/ice":
			fmt.Fprint(w, `{"exchangeResponse":"[]"}`)
		case r.URL.Path == "/v5/sessions/home/timeout" && r.Method == http.MethodDelete:
			stops.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := streaming.NewClient(server.URL, "token", "home", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := Start(ctx, client, Options{Play: streaming.PlayOptions{Target: "console"}, ConnectionTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	select {
	case <-s.Done():
		if !errors.Is(s.Err(), ErrConnectionTimeout) {
			t.Fatalf("unexpected session cause: %v", s.Err())
		}
	case <-ctx.Done():
		t.Fatal("session did not time out")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if stops.Load() != 1 {
		t.Fatalf("remote session stopped %d times", stops.Load())
	}
}

func TestSignalingTimeoutCleansUp(t *testing.T) {
	var stops atomic.Int32
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v5/sessions/home/play":
			fmt.Fprint(w, `{"sessionPath":"/v5/sessions/slow"}`)
		case r.URL.Path == "/v5/sessions/home/slow/state":
			fmt.Fprint(w, `{"state":"Provisioned"}`)
		case r.URL.Path == "/v5/sessions/home/slow/sdp":
			if r.Method == http.MethodGet {
				polls.Add(1)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/v5/sessions/home/slow" && r.Method == http.MethodDelete:
			stops.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := streaming.NewClient(server.URL, "token", "home", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := Start(ctx, client, Options{Play: streaming.PlayOptions{Target: "console"}, SignalingTimeout: 500 * time.Millisecond})
	if s != nil || !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("expected signaling timeout, got session %v and error %v", s, err)
	}
	if stops.Load() != 1 || polls.Load() == 0 {
		t.Fatalf("stops=%d signaling polls=%d", stops.Load(), polls.Load())
	}
}

func TestRemoteCandidatesTryAllPaths(t *testing.T) {
	failure := errors.New("candidate rejected")
	candidates := []streaming.ICECandidate{
		{Candidate: "bad", SDPMLineIndex: "0"},
		{Candidate: "good", SDPMid: "video", SDPMLineIndex: "2"},
		{Candidate: "a=end-of-candidates", SDPMLineIndex: "0"},
	}
	var attempted []string
	err := addRemoteCandidates(candidates, func(candidate, mid string, line uint16) error {
		attempted = append(attempted, candidate)
		if candidate == "bad" {
			return failure
		}
		if mid != "video" || line != 2 {
			t.Fatalf("lost media identity: %s %d", mid, line)
		}
		return nil
	})
	if err != nil || len(attempted) != 2 || attempted[1] != "good" {
		t.Fatalf("attempted=%v error=%v", attempted, err)
	}
	err = addRemoteCandidates(candidates[:1], func(string, string, uint16) error { return failure })
	if !errors.Is(err, failure) {
		t.Fatalf("lost failure: %v", err)
	}
	if err = addRemoteCandidates(nil, func(string, string, uint16) error { t.Fatal("unexpected candidate"); return nil }); err != nil {
		t.Fatalf("empty exchange rejected: %v", err)
	}
}
