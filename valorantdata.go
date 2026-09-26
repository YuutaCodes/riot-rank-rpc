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

func fetchWeaponSkins() (map[string]interface{}, error) {
	resp, err := http.DefaultClient.Get("https://valorant-api.com/v1/weapons/skins")
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func buildSkinNameIndex(skins []interface{}) map[string]string {
	index := map[string]string{}

	for _, s := range skins {
		skin := s.(map[string]interface{})
		displayName := skin["displayName"].(string)
		levels := skin["levels"].([]interface{})
		for _, l := range levels {
			level := l.(map[string]interface{})
			uuid := level["uuid"].(string)
			index[uuid] = displayName
		}
	}

	return index
}
