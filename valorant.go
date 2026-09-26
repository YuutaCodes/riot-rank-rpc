package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var localClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

type EntitlementsToken struct {
	AccessToken string
	Token       string
	Subject     string
	Issuer      string
}

func fetchEntitlements(lf Lockfile) (EntitlementsToken, error) {
	url := fmt.Sprintf("https://127.0.0.1:%d/entitlements/v1/token", lf.Port)
	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return EntitlementsToken{}, err
	}

	req.SetBasicAuth("riot", lf.Password)

	resp, err := localClient.Do(req)
	if err != nil {
		return EntitlementsToken{}, err
	}

	defer resp.Body.Close()

	var result EntitlementsToken
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return EntitlementsToken{}, err
	}

	return result, nil
}

type RegionLocale struct {
	Region      string
	Locale      string
	WebLanguage string
}

func fetchRegionLocale(lf Lockfile) (RegionLocale, error) {
	url := fmt.Sprintf("https://127.0.0.1:%d/riotclient/region-locale", lf.Port)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return RegionLocale{}, err
	}

	req.SetBasicAuth("riot", lf.Password)

	resp, err := localClient.Do(req)
	if err != nil {
		return RegionLocale{}, err
	}

	defer resp.Body.Close()

	var result RegionLocale
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return RegionLocale{}, err
	}

	return result, nil
}

// clientPlatform is the base64-encoded JSON blob Valorant's PD/GLZ servers expect describing
// the caller's platform. It's fixed (community-documented), unlike ClientVersion below, which
// changes every game patch and has to be fetched live.
const clientPlatform = "ew0KCSJwbGF0Zm9ybVR5cGUiOiAiUEMiLA0KCSJwbGF0Zm9ybU9TIjogIldpbmRvd3MiLA0KCSJwbGF0Zm9ybU9TVmVyc2lvbiI6ICIxMC4wLjE5MDQyLjEuMjU2LjY0Yml0IiwNCgkicGxhdGZvcm1DaGlwc2V0IjogIlVua25vd24iDQp9"

func fetchMMR(shard string, puuid string, accessToken string, entitlementToken string) (map[string]interface{}, error) {
	clientVersion, err := fetchClientVersion()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://pd.%s.a.pvp.net/mmr/v1/players/%s", shard, puuid)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Riot-Entitlements-JWT", entitlementToken)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	req.Header.Set("X-Riot-ClientVersion", clientVersion)
	resp, err := http.DefaultClient.Do(req)
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

// regionToShard maps Riot's account-region codes (as returned by the local
// region-locale endpoint, e.g. "EUW") to Valorant's actual shard codes used
// in PD/GLZ server URLs ("eu", "na", "ap", "kr"). This is a best-guess table
// from community documentation, NOT verified against a real Valorant client.
// confirm and adjust once tested on a machine with Valorant installed.
var regionToShard = map[string]string{
	"EUW":   "eu",
	"EUNE":  "eu",
	"NA":    "na",
	"LATAM": "na",
	"BR":    "br",
	"KR":    "kr",
	"AP":    "ap",
	"OCE":   "ap",
	"JP":    "ap",
}

func shardFromRegion(region string) string {
	if shard, ok := regionToShard[strings.ToUpper(region)]; ok {
		return shard
	}
	return strings.ToLower(region)
}

type CurrentPartyIDResponse struct {
	CurrentPartyID string `json:"CurrentPartyId"`
}

func fetchPartyID(shard string, puuid string, accessToken string, EntitlementToken string) (string, error) {
	url := fmt.Sprintf("https://glz-%s-1.%s.a.pvp.net/parties/v1/players/%s", shard, shard, puuid)

	clientVersion, err := readClientVersionFromLog()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Riot-Entitlements-JWT", EntitlementToken)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	req.Header.Set("X-Riot-ClientVersion", clientVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	var result CurrentPartyIDResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	return result.CurrentPartyID, nil
}

func readClientVersionFromLog() (string, error) {
	path := os.Getenv("LOCALAPPDATA") + `\VALORANT\Saved\Logs\ShooterGame.log`

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, "CI server version:") {
			parts := strings.Split(line, "CI server version:")
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", errors.New("version line not found in log")
}

func fetchParty(shard string, partyID string, accessToken string, entitlementToken string) (map[string]interface{}, error) {
	clientVersion, err := fetchClientVersion()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://glz-%s-1.%s.a.pvp.net/parties/v1/parties/%s", shard, shard, partyID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Riot-Entitlements-JWT", entitlementToken)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	req.Header.Set("X-Riot-ClientVersion", clientVersion)
	resp, err := http.DefaultClient.Do(req)
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

type MatchIDResponse struct {
	MatchID string `json:"MatchID"`
}

func fetchPregameMatchID(shard string, puuid string, accessToken string, entitlementToken string) (MatchIDResponse, error) {
	clientVersion, err := readClientVersionFromLog()
	if err != nil {
		return MatchIDResponse{}, err
	}

	url := fmt.Sprintf("https://glz-%s-1.%s.a.pvp.net/pregame/v1/players/%s", shard, shard, puuid)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return MatchIDResponse{}, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Riot-Entitlements-JWT", entitlementToken)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	req.Header.Set("X-Riot-ClientVersion", clientVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return MatchIDResponse{}, err
	}

	defer resp.Body.Close()

	var result MatchIDResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return MatchIDResponse{}, err
	}

	return result, nil
}

func fetchPregameMatch(shard string, matchID string, accessToken string, entitlementToken string) (map[string]interface{}, error) {
	clientVersion, err := readClientVersionFromLog()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://glz-%s-1.%s.a.pvp.net/pregame/v1/matches/%s", shard, shard, matchID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Riot-Entitlements-JWT", entitlementToken)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	req.Header.Set("X-Riot-ClientVersion", clientVersion)
	resp, err := http.DefaultClient.Do(req)
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

func fetchCoreGameMatchID(shard string, puuid string, accessToken string, entitlementToken string) (MatchIDResponse, error) {
	clientVersion, err := readClientVersionFromLog()
	if err != nil {
		return MatchIDResponse{}, err
	}

	url := fmt.Sprintf("https://glz-%s-1.%s.a.pvp.net/core-game/v1/players/%s", shard, shard, puuid)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return MatchIDResponse{}, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Riot-Entitlements-JWT", entitlementToken)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	req.Header.Set("X-Riot-ClientVersion", clientVersion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return MatchIDResponse{}, err
	}

	defer resp.Body.Close()

	var result MatchIDResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return MatchIDResponse{}, err
	}

	return result, nil
}

func fetchLoadouts(shard string, matchID string, accessToken string, entitlementToken string) (map[string]interface{}, error) {
	clientVersion, err := readClientVersionFromLog()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://glz-%s-1.%s.a.pvp.net/core-game/v1/matches/%s/loadouts", shard, shard, matchID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Riot-Entitlements-JWT", entitlementToken)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	req.Header.Set("X-Riot-ClientVersion", clientVersion)
	resp, err := http.DefaultClient.Do(req)
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
