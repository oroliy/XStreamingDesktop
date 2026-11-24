package settings

import (
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "dir", "settings.json")
	s, err := Load(p)
	if err != nil || s != Defaults() {
		t.Fatal(s, err)
	}
	s.Resolution = "1080p HQ"
	s.Language = "zh-TW"
	if err = Save(p, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got != s || got.ResolutionCode() != 1081 {
		t.Fatal(got, err)
	}
	s.Market = "invalid"
	if Save(p, s) == nil {
		t.Fatal("accepted invalid market")
	}
}
