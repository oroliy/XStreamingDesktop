package settings

import (
	"math"
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
	s.StickDeadzone = 25
	s.PollingRate = "250 Hz"
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
	if err != nil || got.Volume != 100 || got.StickDeadzone != 10 || got.PollingRate != "60 Hz" || got.ResolutionCode() != 720 || got.UILanguage != "English" {
		t.Fatal(got, err)
	}
	got.Volume = 101
	if err = got.Validate(); err == nil {
		t.Fatal("accepted invalid volume")
	}
}

func TestInvalidStickDeadzone(t *testing.T) {
	s := Defaults()
	for _, value := range []float64{-1, 51, math.NaN(), math.Inf(1)} {
		s.StickDeadzone = value
		if s.Validate() == nil {
			t.Errorf("accepted %v", value)
		}
	}
}

func TestPollingRateValidation(t *testing.T) {
	s := Defaults()
	for value, expected := range map[string]int{"60 Hz": 60, "125 Hz": 125, "250 Hz": 250} {
		s.PollingRate = value
		if err := s.Validate(); err != nil || s.PollingRateHz() != expected {
			t.Fatalf("%s: %v", value, err)
		}
	}
	s.PollingRate = "10000 Hz"
	if s.Validate() == nil {
		t.Fatal("accepted invalid polling rate")
	}
}
