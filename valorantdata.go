package main

import (
	"encoding/json"
	"net/http"
)

// fetchClientVersion returns valorant-api.com's "riotClientVersion" field — despite the name,
// this is the Riot Client's own version, not Valorant's specific build/session version.
func fetchClientVersion() (string, error) {
	resp, err := http.DefaultClient.Get("https://valorant-api.com/v1/version")
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	var result struct {
		Data struct {
			RiotClientVersion string `json:"riotClientVersion"`
		} `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	return result.Data.RiotClientVersion, nil
}

// TODO(human): implement fetchWeaponSkins to explore the shape.
