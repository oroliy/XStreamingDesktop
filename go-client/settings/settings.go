// Package settings persists non-secret preview preferences.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

type Settings struct {
	Resolution string  `json:"resolution"`
	UILanguage string  `json:"ui_language"`
	Language   string  `json:"language"`
	Market     string  `json:"market"`
	Volume     float64 `json:"volume"`
	PreferIPv6 bool    `json:"prefer_ipv6"`
}

func Defaults() Settings {
	return Settings{Resolution: "1080p", UILanguage: "English", Language: "en-US", Market: "US", Volume: 100}
}
func (s Settings) Validate() error {
	if s.UILanguage != "English" && s.UILanguage != "简体中文" {
		return errors.New("invalid interface language")
	}
	if math.IsNaN(s.Volume) || math.IsInf(s.Volume, 0) || s.Volume < 0 || s.Volume > 100 {
		return errors.New("invalid volume")
	}
	switch s.Resolution {
	case "720p", "1080p", "1080p HQ":
	default:
		return errors.New("invalid resolution")
	}
	switch s.Language {
	case "en-US", "zh-TW", "zh-CN", "ja-JP":
	default:
		return errors.New("invalid language")
	}
	switch s.Market {
	case "US", "TW", "JP", "GB":
	default:
		return errors.New("invalid market")
	}
	return nil
}
func (s Settings) ResolutionCode() int {
	switch s.Resolution {
	case "720p":
		return 720
	case "1080p HQ":
		return 1081
	default:
		return 1080
	}
}
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "xstreaming-go", "settings.json"), nil
}
func Load(path string) (Settings, error) {
	s := Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(data, &s); err != nil {
		return Defaults(), err
	}
	if err = s.Validate(); err != nil {
		return Defaults(), err
	}
	return s, nil
}
func Save(path string, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}
