# Go client (early development)

This directory contains an early MyGo migration. The native window is an experimental preview that can initiate home and cloud streaming and display decoded H.264 frames. It reads up to four gamepads with adjustable stick dead zone and polling rate; vibration and other existing features are not implemented. Input, audio and video have not been validated with a real Xbox. Use the original Electron application for streaming.

[中文说明](README.zh_CN.md)

Requires Go 1.27.3 or later and the platform dependencies documented by [MyGo](https://github.com/egoist/mygo). The preview also requires `ffmpeg` and `ffplay` on PATH for video and audio.

```sh
go test ./...
go run ./cmd/xstreaming
```

The `streaming` package currently implements session creation, state polling, keepalive, stop, and SDP/ICE signaling requests using an existing streaming token. ICE candidate normalization is available separately; streaming token refresh retries once after HTTP 401 or 403. The `auth` package implements Microsoft device-code and refresh-token requests plus Xbox user, XSTS and streaming token exchange. The preview has device-code sign-in, console and cloud game lists; refresh tokens are stored in the system credential store where available and rotated when refreshed; the preview supports restoring and removing the saved login. Real account integration has not yet been verified. Its tests use a local HTTP server and do not require an Xbox account.

The `peer` package provides a Pion WebRTC peer with audio/video transceivers and Xbox data channels. Local tests negotiate a peer connection, send a data-channel message, and receive a video RTP packet. The `session` package connects service signaling to the peer and handles provisioning and cleanup; it is connected to the preview UI but has not been tested with an Xbox.

The experimental `media` package can depacketize H.264 RTP and decode H.264 video frames using an `ffmpeg` executable on PATH. The preview UI uses this decoder; it uses software decoding and has not been benchmarked for streaming latency. Opus audio uses `ffplay` and an available audio device; audio/video synchronization and latency have not been validated.

The preview reads up to four controllers through MyGo’s gamepad plugin. Linux requires permission to read the relevant `/dev/input` device. Polling-rate changes apply to the next stream; higher rates increase processing and network activity. Rumble is not yet supported.

Cloud catalog market and language and streaming resolution are configurable in the preview. The catalog reloads automatically after changing market or language. The market affects product metadata, not account entitlement. The cloud streaming region can be selected separately from those returned for the account. The game list supports search and pagination. If a catalog batch fails, successfully loaded names remain visible and other entries use product IDs. Cloud access depends on account entitlements and region.

Login persistence uses MyGo Secrets (Keychain, Windows Credential Manager or Linux Secret Service). If unavailable, sign-in works for the current run without persistence.

GitHub Actions uploads unsigned preview binaries as short-lived workflow artifacts for supported targets. They do not include FFmpeg or ffplay. On macOS, GUI apps may inherit a different PATH from a terminal; if the executables are not found, start the preview from a terminal where they are available. On macOS and Linux, run `chmod +x xstreaming` after extracting the artifact.

During a stream, keyboard gamepad mode can be enabled manually. WASD and TFGH control the sticks, IJKL the face buttons, arrow keys the D-pad, 1/4 triggers, 2/3 shoulders; Esc disables keyboard mode.

The stream view offers a full-screen toggle and displays decoded video frames per second (which may differ from rendered frames). Starting volume is configurable before a stream; changes apply to the next stream.

The console list can send wake and power-off requests through Xbox remote management. An optional setting sends a wake request for a standby console before starting a home stream. During a home stream, text can be sent to the console while keyboard gamepad mode is disabled; the stream can also be disconnected and then powered off after session cleanup. The console must allow remote management, and an accepted request does not guarantee the power state has changed. This flow has not been tested with a real Xbox.

The optional IPv6 preference orders remote ICE candidates for new sessions; IPv4 candidates remain available.

The interface language can be switched between English and Simplified Chinese independently of the game language.
