package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolve(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("market") != "US" || r.Header.Get("calling-app-name") != "Xbox Cloud Gaming Web" {
			t.Error("bad request")
		}
		var body struct{ Products []string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Products) > 100 {
			t.Error("batch too big")
		}
		fmt.Fprint(w, `{"Products":{"ID0":{"ProductTitle":"A Game"}}}`)
	}))
	defer s.Close()
	titles := make([]streaming.Title, 101)
	for i := range titles {
		titles[i].TitleID = fmt.Sprint(i)
		titles[i].Details.ProductID = fmt.Sprintf("id%d", i)
	}
	got, err := (&Client{http: s.Client(), endpoint: s.URL}).Resolve(context.Background(), titles, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(got) != 101 || got[0].Name != "A Game" {
		t.Fatalf("calls=%d titles=%d first=%+v", calls, len(got), got[0])
	}
}

func TestResolveCloudIDFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Products":{"ABC":{"ProductTitle":"Game","XCloudTitleId":"catalog-id"}}}`)
	}))
	defer server.Close()
	title := streaming.Title{}
	title.Details.ProductID = "abc"
	out, err := (&Client{http: server.Client(), endpoint: server.URL}).Resolve(context.Background(), []streaming.Title{title}, "", "")
	if err != nil || len(out) != 1 || out[0].TitleID != "catalog-id" {
		t.Fatal(out, err)
	}
	title.TitleID = "service-id"
	out, err = (&Client{http: server.Client(), endpoint: server.URL}).Resolve(context.Background(), []streaming.Title{title}, "", "")
	if err != nil || out[0].TitleID != "service-id" {
		t.Fatal(out, err)
	}
}

func TestMissingProducts(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer s.Close()
	var title streaming.Title
	title.Details.ProductID = "id"
	if _, err := (&Client{http: s.Client(), endpoint: s.URL}).Resolve(context.Background(), []streaming.Title{title}, "", ""); err == nil {
		t.Fatal("accepted missing products")
	}
}
