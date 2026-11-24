// Package catalog resolves Game Pass product names for streaming titles.
package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
)

type Client struct {
	http     *http.Client
	endpoint string
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}, endpoint: "https://catalog.gamepass.com/v3/products"}
}

// Resolve enriches known title IDs with catalog names, preserving entries absent from the catalog.
func (c *Client) Resolve(ctx context.Context, titles []streaming.Title, market, language string) ([]streaming.Title, error) {
	if market == "" {
		market = "US"
	}
	if language == "" {
		language = "en-US"
	}
	out := append([]streaming.Title(nil), titles...)
	ids := make([]string, 0, len(out))
	for _, t := range out {
		if t.Details.ProductID != "" {
			ids = append(ids, t.Details.ProductID)
		}
	}
	names := map[string]string{}
	for start := 0; start < len(ids); start += 100 {
		end := min(start+100, len(ids))
		b, _ := json.Marshal(map[string]any{"Products": ids[start:end]})
		u, err := url.Parse(c.endpoint)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("market", market)
		q.Set("language", language)
		q.Set("hydration", "RemoteLowJade0")
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("calling-app-name", "Xbox Cloud Gaming Web")
		req.Header.Set("calling-app-version", "24.17.63")
		req.Header.Set("ms-cv", "0")
		res, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, fmt.Errorf("catalog returned HTTP %d", res.StatusCode)
		}
		var payload struct {
			Products map[string]struct {
				ProductTitle  string `json:"ProductTitle"`
				XCloudTitleID string `json:"XCloudTitleId"`
			} `json:"Products"`
		}
		if err = json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		for id, product := range payload.Products {
			names[strings.ToUpper(id)] = product.ProductTitle
		}
	}
	for i := range out {
		out[i].Name = names[strings.ToUpper(out[i].Details.ProductID)]
		if out[i].Name == "" {
			out[i].Name = out[i].Details.ProductID
		}
		if out[i].Name == "" {
			out[i].Name = out[i].TitleID
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
