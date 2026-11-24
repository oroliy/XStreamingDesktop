package settings

import (
	"os"
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

func TestLegacySettingsGainDefaultVolume(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte(`{"resolution":"720p","language":"en-US","market":"US"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got.Volume != 100 || got.ResolutionCode() != 720 || got.UILanguage != "English" {
		t.Fatal(got, err)
	}
	got.Volume = 101
	if err = got.Validate(); err == nil {
		t.Fatal("accepted invalid volume")
	}
}
