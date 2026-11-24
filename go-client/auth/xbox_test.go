package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestXboxTokenExchange(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/user":
			props := body["Properties"].(map[string]any)
			if props["RpsTicket"] != "d=access" {
				t.Error("bad ticket")
			}
			w.Write([]byte(`{"Token":"user"}`))
		case "/xsts":
			if body["RelyingParty"] != "http://gssv.xboxlive.com/" {
				t.Error("bad relying party")
			}
			w.Write([]byte(`{"Token":"xsts"}`))
		case "/stream":
			if body["token"] != "xsts" || body["offeringId"] != "xhome" || r.Header.Get("x-gssv-client") != "XboxComBrowser" {
				t.Error("bad streaming request")
			}
			w.Write([]byte(`{"gsToken":"gs","offeringSettings":{"regions":[{"name":"a","baseUri":"https://example.com","isDefault":true}]}}`))
		default:
			t.Error("unexpected request")
		}
	}))
	defer s.Close()
	c := &XboxClient{http: s.Client(), userURL: s.URL + "/user", xstsURL: s.URL + "/xsts", homeURL: s.URL + "/stream"}
	ctx := context.Background()
	user, err := c.UserToken(ctx, "access")
	if err != nil {
		t.Fatal(err)
	}
	xsts, err := c.XSTS(ctx, user.Token, "http://gssv.xboxlive.com/")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := c.Streaming(ctx, xsts.Token, "xhome")
	if err != nil {
		t.Fatal(err)
	}
	region, err := stream.DefaultRegion()
	if err != nil || stream.GSToken != "gs" || region.Name != "a" {
		t.Fatal("invalid token or region", err)
	}
}
func TestXboxError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"XErr":2148916233}`))
	}))
	defer s.Close()
	c := &XboxClient{http: s.Client(), userURL: s.URL}
	_, err := c.UserToken(context.Background(), "access")
	var service *ServiceError
	if !errors.As(err, &service) || service.Status != 401 || service.XErr != 2148916233 {
		t.Fatal(err)
	}
}
