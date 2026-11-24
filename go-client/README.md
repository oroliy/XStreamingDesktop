# Go client (early development)

This directory contains the beginning of the MyGo migration. The native window is a preview only: WebRTC audio/video, controller input, and other existing features are not implemented. Use the original Electron application for streaming.

Requires Go 1.27.3 or later and the platform dependencies documented by [MyGo](https://github.com/egoist/mygo).

```sh
go test ./...
go run ./cmd/xstreaming
```

The `streaming` package currently implements session creation, state polling, keepalive, stop, and SDP/ICE signaling requests using an existing streaming token. ICE candidate normalization is available separately; streaming token refresh is not yet implemented. The `auth` package implements Microsoft device-code and refresh-token requests plus Xbox user, XSTS and streaming token exchange. The preview has a device-code sign-in and console list; automatic token refresh and persistent login are not yet implemented. Real account integration has not yet been verified. Its tests use a local HTTP server and do not require an Xbox account.

The `peer` package provides a Pion WebRTC peer with audio/video transceivers and Xbox data channels. Local tests negotiate a peer connection, send a data-channel message, and receive a video RTP packet. The `session` package connects service signaling to the peer and handles provisioning and cleanup; it is not yet wired into the UI and does not decode or play media.

The experimental `media` package can depacketize H.264 RTP and decode H.264 video frames using an `ffmpeg` executable on PATH. This decoder is not connected to the UI yet; it uses software decoding and has not been benchmarked for streaming latency. Audio remains unimplemented.
