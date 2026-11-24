# Go client (early development)

This directory contains an early MyGo migration. The native window is an experimental preview that can initiate home and cloud streaming and display decoded H.264 frames. It reads the first gamepad; vibration and other existing features are not implemented. Input, audio and video have not been validated with a real Xbox. Use the original Electron application for streaming.

[中文说明](README.zh_CN.md)

Requires Go 1.27.3 or later and the platform dependencies documented by [MyGo](https://github.com/egoist/mygo). The preview also requires `ffmpeg` and `ffplay` on PATH for video and audio.

```sh
go test ./...
go run ./cmd/xstreaming
```

The `streaming` package currently implements session creation, state polling, keepalive, stop, and SDP/ICE signaling requests using an existing streaming token. ICE candidate normalization is available separately; streaming token refresh retries once after HTTP 401 or 403. The `auth` package implements Microsoft device-code and refresh-token requests plus Xbox user, XSTS and streaming token exchange. The preview has device-code sign-in, console and cloud game lists; refresh tokens are stored in the system credential store where available and rotated when refreshed; the preview supports restoring and removing the saved login. Real account integration has not yet been verified. Its tests use a local HTTP server and do not require an Xbox account.

The `peer` package provides a Pion WebRTC peer with audio/video transceivers and Xbox data channels. Local tests negotiate a peer connection, send a data-channel message, and receive a video RTP packet. The `session` package connects service signaling to the peer and handles provisioning and cleanup; it is connected to the preview UI but has not been tested with an Xbox.

The experimental `media` package can depacketize H.264 RTP and decode H.264 video frames using an `ffmpeg` executable on PATH. The preview UI uses this decoder; it uses software decoding and has not been benchmarked for streaming latency. Opus audio uses `ffplay` and an available audio device; audio/video synchronization and latency have not been validated.

The preview reads the first controller through MyGo’s gamepad plugin. Linux requires permission to read the relevant `/dev/input` device. Rumble and multiple controllers are not yet supported.

Cloud catalog market and language and streaming resolution are configurable in the preview. Reload the catalog after changing market or language. If catalog lookup fails, the preview displays product IDs. Cloud access depends on account entitlements and region.

Login persistence uses MyGo Secrets (Keychain, Windows Credential Manager or Linux Secret Service). If unavailable, sign-in works for the current run without persistence.

GitHub Actions uploads unsigned preview binaries as short-lived workflow artifacts for supported targets. They do not include FFmpeg or ffplay. On macOS and Linux, run `chmod +x xstreaming` after extracting the artifact.
