# Go client (early development)

This directory contains the beginning of the MyGo migration. The native window is an experimental preview: it can initiate home and cloud streaming and display decoded H.264 frames, but physical controller polling is implemented for the first gamepad, but vibration and other existing features are not. Input, audio and video have not been validated with a real Xbox. Use the original Electron application for streaming.

Requires Go 1.27.3 or later and the platform dependencies documented by [MyGo](https://github.com/egoist/mygo).

```sh
go test ./...
go run ./cmd/xstreaming
```

The `streaming` package currently implements session creation, state polling, keepalive, stop, and SDP/ICE signaling requests using an existing streaming token. ICE candidate normalization is available separately; streaming token refresh retries once after HTTP 401 or 403. The `auth` package implements Microsoft device-code and refresh-token requests plus Xbox user, XSTS and streaming token exchange. The preview has device-code sign-in, console and cloud game lists; tokens refresh in memory when needed; persistent login is not yet implemented. Real account integration has not yet been verified. Its tests use a local HTTP server and do not require an Xbox account.

The `peer` package provides a Pion WebRTC peer with audio/video transceivers and Xbox data channels. Local tests negotiate a peer connection, send a data-channel message, and receive a video RTP packet. The `session` package connects service signaling to the peer and handles provisioning and cleanup; it is connected to the preview UI but has not been tested with an Xbox.

The experimental `media` package can depacketize H.264 RTP and decode H.264 video frames using an `ffmpeg` executable on PATH. The preview UI uses this decoder; it uses software decoding and has not been benchmarked for streaming latency. Opus audio uses `ffplay` and an available audio device; audio/video synchronization and latency have not been validated.

The preview reads the first controller through MyGo’s gamepad plugin. Linux requires permission to read the relevant `/dev/input` device. Rumble and multiple controllers are not yet supported.

Cloud catalog names currently use the US market and English. If catalog lookup fails, the preview displays product IDs. Cloud access depends on account entitlements and region.
