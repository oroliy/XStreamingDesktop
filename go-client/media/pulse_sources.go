package media

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// PulseSources lists PulseAudio source names using the existing FFmpeg dependency.
// It does not open or record from a source.
func PulseSources(ctx context.Context) ([]string, error) {
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

func parsePulseSources(output string) ([]string, error) {
	if !strings.Contains(output, "Auto-detected sources for pulse:") || strings.Contains(output, "Cannot list sources:") {
		return nil, errors.New("PulseAudio inputs unavailable")
	}
	seen := make(map[string]bool)
	var sources []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		name, _, ok := strings.Cut(line, " [")
		if !ok || !validPulseName(name) || seen[name] {
			continue
		}
		seen[name] = true
		sources = append(sources, name)
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
