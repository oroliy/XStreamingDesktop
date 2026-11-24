// Package streaming implements the Xbox streaming service HTTP protocol.
package streaming

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	mu        sync.Mutex
	refreshMu sync.Mutex
	refresh   func(context.Context) (string, string, error)
	base      *url.URL
	token     string
	kind      string
	http      *http.Client
}

// NewClient accepts the regional streaming service URL and its gsToken.
func NewClient(baseURL, token, kind string, client *http.Client) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("invalid streaming service URL")
	}
	if token == "" || (kind != "home" && kind != "cloud") {
		return nil, fmt.Errorf("invalid token or session type")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{base: u, token: token, kind: kind, http: client}, nil
}

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("streaming service returned HTTP %d", e.Status)
}

func (c *Client) request(ctx context.Context, method, path string, body any, out any) (int, error) {
	return c.requestHeaders(ctx, method, path, body, out, nil)
}

// SetRefresh configures a token and regional URL refresh after an authentication failure.
func (c *Client) SetRefresh(fn func(context.Context) (string, string, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refresh = fn
}
func (c *Client) requestHeaders(ctx context.Context, method, path string, body any, out any, headers http.Header) (int, error) {
	c.mu.Lock()
	old := c.token
	c.mu.Unlock()
	status, err := c.requestOnce(ctx, method, path, body, out, headers)
	if status != 401 && status != 403 {
		return status, err
	}
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	c.mu.Lock()
	current, fn := c.token, c.refresh
	c.mu.Unlock()
	if fn == nil {
		return status, err
	}
	if current == old {
		token, base, e := fn(ctx)
		if e != nil {
			return status, fmt.Errorf("refresh streaming token: %w", e)
		}
		validated, e := NewClient(base, token, c.kind, c.http)
		if e != nil {
			return status, e
		}
		c.mu.Lock()
		if c.base.Scheme == "https" && validated.base.Scheme != "https" {
			c.mu.Unlock()
			return status, fmt.Errorf("refusing insecure streaming region")
		}
		c.token = validated.token
		c.base = validated.base
		c.mu.Unlock()
	}
	return c.requestOnce(ctx, method, path, body, out, headers)
}
func (c *Client) requestOnce(ctx context.Context, method, path string, body any, out any, headers http.Header) (int, error) {
	var data io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		data = bytes.NewReader(b)
	}
	c.mu.Lock()
	u := *c.base
	token := c.token
	c.mu.Unlock()
	u.Path, u.RawQuery, _ = strings.Cut(path, "?")
	req, err := http.NewRequestWithContext(ctx, method, u.String(), data)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, &HTTPError{resp.StatusCode, string(b)}
	}
	if out != nil && len(bytes.TrimSpace(b)) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

func (c *Client) sessionPath(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, "/?#") || id == "." || id == ".." {
		return "", fmt.Errorf("invalid session ID")
	}
	return "/v5/sessions/" + c.kind + "/" + id, nil
}

// State reads a session's state, polling on HTTP 204 until a response or cancellation.
func (c *Client) State(ctx context.Context, id string) (json.RawMessage, error) {
	path, err := c.sessionPath(id)
	if err != nil {
		return nil, err
	}
	for {
		var result json.RawMessage
		status, err := c.request(ctx, http.MethodGet, path+"/state", nil, &result)
		if err != nil {
			return nil, err
		}
		if status != http.StatusNoContent {
			return result, nil
		}
		t := time.NewTimer(750 * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}
func (c *Client) Keepalive(ctx context.Context, id string) error {
	path, err := c.sessionPath(id)
	if err != nil {
		return err
	}
	_, err = c.request(ctx, http.MethodPost, path+"/keepalive", struct{}{}, nil)
	return err
}
func (c *Client) Stop(ctx context.Context, id string) error {
	path, err := c.sessionPath(id)
	if err != nil {
		return err
	}
	_, err = c.request(ctx, http.MethodDelete, path, nil, nil)
	return err
}
