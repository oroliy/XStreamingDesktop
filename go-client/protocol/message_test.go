package protocol

import (
	"encoding/json"
	"testing"
)

func TestHandshake(t *testing.T) {
	m := NewMessageChannel()
	hello := m.Handshake()
	if hello.Type != "Handshake" || hello.Version != "messageV1" || hello.ID == "" || hello.CV == "" {
		t.Fatal(hello)
	}
	if _, err := m.HandleHandshake([]byte(`{"type":"HandshakeAck","version":"bad"}`)); err == nil {
		t.Fatal("invalid version accepted")
	}
	out, err := m.HandleHandshake([]byte(`{"type":"HandshakeAck","version":"messageV1"}`))
	if err != nil || len(out) != 6 {
		t.Fatal(out, err)
	}
	if out[0].Target != "/streaming/systemUi/configuration" {
		t.Fatal(out[0])
	}
	var config struct {
		Version []int `json:"version"`
	}
	if err := json.Unmarshal([]byte(out[0].Content), &config); err != nil || len(config.Version) != 3 {
		t.Fatal(err)
	}
	out, err = m.HandleHandshake([]byte(`{"type":"HandshakeAck","version":"messageV1"}`))
	if err != nil || len(out) != 0 {
		t.Fatal(out, err)
	}
}
func TestControlMessages(t *testing.T) {
	if Authorization()["message"] != "authorizationRequest" || GamepadChanged(0, true)["wasAdded"] != true || KeyframeRequest()["ifrRequested"] != true {
		t.Fatal("invalid control message")
	}
}
