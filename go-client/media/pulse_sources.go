package media

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

type PulseSource struct {
	Name        string
	Description string
}

// PulseSources lists PulseAudio sources using the existing FFmpeg dependency.
// It does not open or record from a source.
func PulseSources(ctx context.Context) ([]PulseSource, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-sources", "pulse")
	configureMediaProcess(cmd)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var output boundedOutput
	var diagnostic diagnosticTail
	cmd.Stdout = &output
	cmd.Stderr = &diagnostic
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, processError("list PulseAudio sources", err, &diagnostic)
	}
	if output.truncated {
		return nil, errors.New("PulseAudio source list is too large")
	}
	return parsePulseSources(output.buf.String())
}

type boundedOutput struct {
	buf       bytes.Buffer
	truncated bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	const limit = 64 << 10
	n := len(p)
	remaining := limit - b.buf.Len()
	if len(p) > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	_, _ = b.buf.Write(p)
	return n, nil
}

func parsePulseSources(output string) ([]PulseSource, error) {
	if !strings.Contains(output, "Auto-detected sources for pulse:") || strings.Contains(output, "Cannot list sources:") {
		return nil, errors.New("PulseAudio inputs unavailable")
	}
	seen := make(map[string]bool)
	var sources []PulseSource
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		name, detail, ok := strings.Cut(line, " [")
		if !ok || !validPulseName(name) || seen[name] {
			continue
		}
		end := strings.LastIndex(detail, "]")
		if end < 0 {
			continue
		}
		description := []rune(strings.TrimSpace(detail[:end]))
		if len(description) > 160 {
			description = description[:160]
		}
		seen[name] = true
		sources = append(sources, PulseSource{Name: name, Description: string(description)})
	}
	return sources, nil
}

func validPulseName(name string) bool {
	if name == "" || len(name) > 256 {
		return false
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '.' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}
