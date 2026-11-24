# Go client (early development)

This directory contains the beginning of the MyGo migration. The native window is a preview only: login, WebRTC audio/video, controller input, and other existing features are not implemented. Use the original Electron application for streaming.

Requires Go 1.27.3 or later and the platform dependencies documented by [MyGo](https://github.com/egoist/mygo).

```sh
go test ./...
go run ./cmd/xstreaming
```

The `streaming` package currently implements session creation, state polling, keepalive, stop, and SDP/ICE signaling requests using an existing streaming token. ICE candidate normalization and token refresh are not yet implemented. Its tests use a local HTTP server and do not require an Xbox account.
