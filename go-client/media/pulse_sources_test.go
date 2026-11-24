package media

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestParsePulseSources(t *testing.T) {
	data := "Auto-detected sources for pulse:\n* alsa_input.usb [USB Microphone] (none)\n  virtual_mic [Null Input] (none)\n  alsa_input.usb [Duplicate] (none)\n  invalid name [Bad] (none)\n"
	sources, err := parsePulseSources(data)
	if err != nil || !reflect.DeepEqual(sources, []string{"alsa_input.usb", "virtual_mic"}) {
		t.Fatalf("sources=%v error=%v", sources, err)
	}
	for _, invalid := range []string{"", "Auto-detected sources for pulse:\nCannot list sources: failed\n"} {
		if _, err := parsePulseSources(invalid); err == nil {
			t.Fatalf("accepted source-list error %q", invalid)
		}
	}
}

func TestPulseSourcesWithVirtualServer(t *testing.T) {
	server := os.Getenv("XSTREAMING_TEST_PULSE_SERVER")
	if server == "" {
		t.Skip("no isolated PulseAudio test server configured")
	}
	t.Setenv("PULSE_SERVER", server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sources, err := PulseSources(ctx)
	if err != nil || !reflect.DeepEqual(sources, []string{"virtual_mic"}) {
		t.Fatalf("sources=%v error=%v", sources, err)
	}
}
