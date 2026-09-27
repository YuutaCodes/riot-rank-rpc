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

// checkStatus errors on non-2xx responses. Riot's error bodies are valid JSON,
// so decoding alone won't catch them.
func checkStatus(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, resp.Request.URL)
	}
	return nil
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

	if resp.StatusCode != http.StatusOK && result.Subject == "" {
		return EntitlementsToken{}, errNotRunning
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

// clientPlatform is a fixed base64 blob that PD and GLZ require.
const clientPlatform = "ew0KCSJwbGF0Zm9ybVR5cGUiOiAiUEMiLA0KCSJwbGF0Zm9ybU9TIjogIldpbmRvd3MiLA0KCSJwbGF0Zm9ybU9TVmVyc2lvbiI6ICIxMC4wLjE5MDQyLjEuMjU2LjY0Yml0IiwNCgkicGxhdGZvcm1DaGlwc2V0IjogIlVua25vd24iDQp9"

func fetchMMR(shard string, puuid string, accessToken string, entitlementToken string) (map[string]any, error) {
	clientVersion, err := cachedClientVersion()
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

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func fetchMatchHistory(shard string, puuid string, accessToken string, entitlementToken string) (map[string]any, error) {
	clientVersion, err := cachedClientVersion()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://pd.%s.a.pvp.net/match-history/v1/history/%s?startIndex=0&endIndex=5", shard, puuid)

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

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func fetchMatchDetails(shard string, matchID string, accessToken string, entitlementToken string) (map[string]any, error) {
	clientVersion, err := cachedClientVersion()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://pd.%s.a.pvp.net/match-details/v1/matches/%s", shard, matchID)

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

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// regionToShard maps a region like "EUW" to a shard like "eu".
// Based on community docs, not verified for every region.
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

	if err := checkStatus(resp); err != nil {
		return "", err
	}

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

func fetchParty(shard string, partyID string, accessToken string, entitlementToken string) (map[string]any, error) {
	clientVersion, err := cachedClientVersion()
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

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

type PlayerName struct {
	Subject  string
	GameName string
	TagLine  string
}

// fetchPlayerNames turns puuids into Riot IDs. Sent as a PUT with a JSON array body.
func fetchPlayerNames(shard string, puuids []string, accessToken string, entitlementToken string) ([]PlayerName, error) {
	clientVersion, err := cachedClientVersion()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://pd.%s.a.pvp.net/name-service/v2/players", shard)

	body, err := json.Marshal(puuids)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PUT", url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Riot-Entitlements-JWT", entitlementToken)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	req.Header.Set("X-Riot-ClientVersion", clientVersion)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result []PlayerName
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

	if err := checkStatus(resp); err != nil {
		return MatchIDResponse{}, err
	}

	var result MatchIDResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return MatchIDResponse{}, err
	}

	return result, nil
}

func fetchPregameMatch(shard string, matchID string, accessToken string, entitlementToken string) (map[string]any, error) {
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

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result map[string]any
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

	if err := checkStatus(resp); err != nil {
		return MatchIDResponse{}, err
	}

	var result MatchIDResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return MatchIDResponse{}, err
	}

	return result, nil
}

func fetchCoreGameMatch(shard string, matchID string, accessToken string, entitlementToken string) (map[string]any, error) {
	clientVersion, err := readClientVersionFromLog()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://glz-%s-1.%s.a.pvp.net/core-game/v1/matches/%s", shard, shard, matchID)

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

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func fetchLoadouts(shard string, matchID string, accessToken string, entitlementToken string) (map[string]any, error) {
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

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
