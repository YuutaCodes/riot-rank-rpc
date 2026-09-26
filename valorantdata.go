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

func fetchWeaponSkins() (map[string]any, error) {
	resp, err := http.DefaultClient.Get("https://valorant-api.com/v1/weapons/skins")
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func buildSkinNameIndex(skins []any) map[string]string {
	index := map[string]string{}

	for _, s := range skins {
		skin := s.(map[string]any)
		displayName := skin["displayName"].(string)

		baseUUID := skin["uuid"].(string)
		index[baseUUID] = displayName

		levels := skin["levels"].([]any)
		for _, l := range levels {
			level := l.(map[string]any)
			uuid := level["uuid"].(string)
			index[uuid] = displayName
		}
	}

	return index
}

func fetchAgents() (map[string]any, error) {
	resp, err := http.DefaultClient.Get("https://valorant-api.com/v1/agents")
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func buildAgentNameIndex(agents []any) map[string]string {
	index := map[string]string{}

	for _, a := range agents {
		agent := a.(map[string]any)
		uuid := agent["uuid"].(string)
		displayName := agent["displayName"].(string)
		index[uuid] = displayName
	}

	return index
}

// skinSocketID is the fixed socket UUID GLZ uses for "which skin is equipped on this weapon"
// confirmed live against a real loadout response. The other socket UUIDs on a weapon item
// (buddy, spray, chroma selection, etc.) aren't resolved here yet.
const skinSocketID = "bcef87d6-209b-46c6-8b19-fbe40bd95abc"

type PlayerLoadout struct {
	Subject   string
	AgentName string
	Skins     []string
}

func summarizeLoadouts(loadouts map[string]any, skinIndex map[string]string, agentIndex map[string]string) []PlayerLoadout {
	var summaries []PlayerLoadout

	players := loadouts["Loadouts"].([]any)
	for _, p := range players {
		player := p.(map[string]any)
		subject := player["Subject"].(string)
		characterID := player["CharacterID"].(string)

		loadout := player["Loadout"].(map[string]any)
		items := loadout["Items"].(map[string]any)

		var skins []string
		for _, i := range items {
			item := i.(map[string]any)
			sockets := item["Sockets"].(map[string]any)

			skinSocket, ok := sockets[skinSocketID].(map[string]any)
			if !ok {
				continue
			}
			skinItem := skinSocket["Item"].(map[string]any)
			skinID := skinItem["ID"].(string)

			if name, ok := skinIndex[skinID]; ok {
				skins = append(skins, name)
			}
		}

		summaries = append(summaries, PlayerLoadout{
			Subject:   subject,
			AgentName: agentIndex[characterID],
			Skins:     skins,
		})
	}

	return summaries
}
