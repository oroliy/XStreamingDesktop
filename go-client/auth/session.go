package auth

import (
	"context"
	"errors"
	"sync"
)

// Session serializes refresh-token rotation across home, cloud and transfer requests.
type Session struct {
	mu        sync.Mutex
	microsoft *Client
	xbox      *XboxClient
	refresh   string
}

func NewSession(refresh string) *Session {
	return &Session{microsoft: NewClient(), xbox: NewXboxClient(), refresh: refresh}
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
		s.refresh = token.RefreshToken
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
		s.refresh = token.RefreshToken
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
