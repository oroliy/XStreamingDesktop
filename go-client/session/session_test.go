package session

import (
	"context"
	"encoding/json"
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
	s, err := Start(ctx, client, Options{Play: streaming.PlayOptions{Target: "console"}})
	if err != nil {
		t.Fatal(err)
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
