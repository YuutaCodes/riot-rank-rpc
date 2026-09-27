package main

import (
	"encoding/json"
	"os"
)

const settingsFile = "settings.json"

// Settings are user choices that persist between runs.
type Settings struct {
	PrimaryWeaponID string   `json:"primaryWeaponID"`
	HiddenColumns   []string `json:"hiddenColumns,omitempty"`
}

// loadSettings reads saved settings, falling back to defaults if the file is missing or invalid.
func loadSettings() Settings {
	defaults := Settings{PrimaryWeaponID: vandalWeaponID}

	data, err := os.ReadFile(settingsFile)
	if err != nil {
		return defaults
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return defaults
	}
	if s.PrimaryWeaponID == "" {
		s.PrimaryWeaponID = defaults.PrimaryWeaponID
	}

	return s
}

func saveSettings(s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsFile, data, 0644)
}
