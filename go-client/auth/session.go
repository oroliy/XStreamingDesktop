package auth

import (
	"context"
	"errors"
	"sync"
)

// Session serializes refresh-token rotation across home, cloud and transfer requests.
type Session struct {
	mu         sync.Mutex
	microsoft  *Client
	xbox       *XboxClient
	refresh    string
	store      TokenStore
	storeError func(error)
	closed     bool
}

func NewSession(refresh string) *Session {
	return &Session{microsoft: NewClient(), xbox: NewXboxClient(), refresh: refresh}
}

// TokenStore holds only the refresh token in a system credential store.
type TokenStore interface {
	Save(string) error
	Delete() error
}

// SetStore saves the current token and persists future token rotation.
func (s *Session) SetStore(store TokenStore, onError func(error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("session closed")
	}
	s.store = store
	s.storeError = onError
	if store == nil || s.refresh == "" {
		return nil
	}
	return store.Save(s.refresh)
}
func (s *Session) rotate(refresh string) {
	if refresh == "" {
		return
	}
	s.refresh = refresh
	if s.store != nil {
		if err := s.store.Save(refresh); err != nil && s.storeError != nil {
			s.storeError(err)
		}
	}
}

// Logout prevents further token refresh and removes the persisted credential.
func (s *Session) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.refresh = ""
	if s.store != nil {
		return s.store.Delete()
	}
	return nil
}
func (s *Session) Transfer(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refresh == "" {
		return "", errors.New("missing refresh token")
	}
	token, err := s.microsoft.TransferToken(ctx, s.refresh)
	if err != nil {
		return "", err
	}
	if token.RefreshToken != "" {
		s.rotate(token.RefreshToken)
	}
	return token.AccessToken, nil
}

// StreamingToken refreshes the Microsoft and Xbox credentials and returns the new regional token.
func (s *Session) StreamingToken(ctx context.Context, kind string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refresh == "" {
		return "", "", errors.New("missing refresh token")
	}
	token, err := s.microsoft.Refresh(ctx, s.refresh)
	if err != nil {
		return "", "", err
	}
	if token.RefreshToken != "" {
		s.rotate(token.RefreshToken)
	}
	user, err := s.xbox.UserToken(ctx, token.AccessToken)
	if err != nil {
		return "", "", err
	}
	xsts, err := s.xbox.XSTS(ctx, user.Token, "http://gssv.xboxlive.com/")
	if err != nil {
		return "", "", err
	}
	offering := "xhome"
	if kind == "cloud" {
		offering = "xgpuweb"
	} else if kind != "home" {
		return "", "", errors.New("invalid streaming type")
	}
	stream, err := s.xbox.Streaming(ctx, xsts.Token, offering)
	if err != nil && kind == "cloud" {
		stream, err = s.xbox.Streaming(ctx, xsts.Token, "xgpuwebf2p")
	}
	if err != nil {
		return "", "", err
	}
	region, err := stream.DefaultRegion()
	if err != nil {
		return "", "", err
	}
	return stream.GSToken, region.BaseURI, nil
}
