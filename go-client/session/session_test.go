package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
	"github.com/pion/webrtc/v4"
	"net/http"
	"net/http/httptest"
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
	var answer string
	var stopped atomic.Int32
	stoppedSignal := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v5/sessions/home/play":
			fmt.Fprint(w, `{"sessionPath":"/v5/sessions/abc"}`)
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
