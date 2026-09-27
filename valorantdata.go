package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
)

var (
	clientVersionMu     sync.Mutex
	clientVersionCached string
)

func cachedClientVersion() (string, error) {
	clientVersionMu.Lock()

	defer clientVersionMu.Unlock()

	if clientVersionCached != "" {
		return clientVersionCached, nil
	}

	v, err := fetchClientVersion()
	if err != nil {
		return "", err
	}

	clientVersionCached = v
	return clientVersionCached, nil
}

// fetchClientVersion returns the Riot Client version, not the exact game build.
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

type SkinInfo struct {
	Name string
	Icon string
}

func buildSkinIndex(skins []any) map[string]SkinInfo {
	index := map[string]SkinInfo{}

	for _, s := range skins {
		skin := s.(map[string]any)
		info := SkinInfo{
			Name: skin["displayName"].(string),
			Icon: skinIconURL(skin),
		}

		baseUUID := skin["uuid"].(string)
		index[baseUUID] = info

		levels := skin["levels"].([]any)
		for _, l := range levels {
			level := l.(map[string]any)
			uuid := level["uuid"].(string)
			index[uuid] = info
		}
	}

	return index
}

// skinIconURL prefers the chroma full render, because Standard skins have a
// placeholder cross as their displayIcon.
func skinIconURL(skin map[string]any) string {
	if chromas, ok := skin["chromas"].([]any); ok && len(chromas) > 0 {
		if chroma, ok := chromas[0].(map[string]any); ok {
			if icon, ok := chroma["fullRender"].(string); ok && icon != "" {
				return icon
			}
		}
	}
	if icon, ok := skin["displayIcon"].(string); ok && icon != "" {
		return icon
	}
	if levels, ok := skin["levels"].([]any); ok && len(levels) > 0 {
		if level, ok := levels[0].(map[string]any); ok {
			if icon, ok := level["displayIcon"].(string); ok && icon != "" {
				return icon
			}
		}
	}
	return ""
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

// skinSocketID is the loadout socket that holds the equipped skin.
const skinSocketID = "bcef87d6-209b-46c6-8b19-fbe40bd95abc"

// vandalWeaponID is the default primary weapon until one is picked in Settings.
const vandalWeaponID = "9c82e19d-4575-0200-1a81-3eacf00cf872"

type PlayerLoadout struct {
	Subject     string
	AgentName   string
	TeamID      string
	Skins       []string
	PrimarySkin string
	// Keyed by weapon UUID.
	WeaponSkins map[string]SkinInfo
}

// WeaponOption is one selectable entry in the Settings tab's weapon list.
type WeaponOption struct {
	Name     string
	UUID     string
	Category string
	Cost     int
}

// weaponCategoryOrder matches the in-game buy menu, left to right.
var weaponCategoryOrder = []string{"Sidearm", "SMG", "Shotgun", "Rifle", "Sniper", "Heavy", "Melee"}

func weaponCategoryRank(category string) int {
	for i, c := range weaponCategoryOrder {
		if c == category {
			return i
		}
	}
	return len(weaponCategoryOrder)
}

func fetchWeapons() (map[string]any, error) {
	resp, err := http.DefaultClient.Get("https://valorant-api.com/v1/weapons")
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

func fetchCompetitiveTiers() (map[string]any, error) {
	resp, err := http.DefaultClient.Get("https://valorant-api.com/v1/competitivetiers")
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

// buildTierNameIndex maps a tier number like 18 to "Diamond 1".
// Uses the last entry in the list, which is the current episode.
func buildTierNameIndex(episodes []any) map[int]string {
	index := map[int]string{}
	if len(episodes) == 0 {
		return index
	}

	current := episodes[len(episodes)-1].(map[string]any)
	tiers := current["tiers"].([]any)

	for _, t := range tiers {
		tier := t.(map[string]any)
		number := int(tier["tier"].(float64))
		name := tier["tierName"].(string)
		index[number] = name
	}

	return index
}

func buildWeaponOptions(weapons []any) []WeaponOption {
	var options []WeaponOption

	for _, w := range weapons {
		weapon := w.(map[string]any)
		option := WeaponOption{
			Name: weapon["displayName"].(string),
			UUID: weapon["uuid"].(string),
		}
		// "EEquippableCategory::Rifle" becomes "Rifle".
		if category, ok := weapon["category"].(string); ok {
			option.Category = strings.TrimPrefix(category, "EEquippableCategory::")
		}
		// Melee has no shopData.
		if shop, ok := weapon["shopData"].(map[string]any); ok {
			if cost, ok := shop["cost"].(float64); ok {
				option.Cost = int(cost)
			}
		}
		options = append(options, option)
	}

	sort.SliceStable(options, func(i, j int) bool {
		ri, rj := weaponCategoryRank(options[i].Category), weaponCategoryRank(options[j].Category)
		if ri != rj {
			return ri < rj
		}
		return options[i].Cost < options[j].Cost
	})

	return options
}

func summarizeLoadouts(loadouts map[string]any, skinIndex map[string]SkinInfo, agentIndex map[string]string, primaryWeaponID string) []PlayerLoadout {
	var summaries []PlayerLoadout

	players := loadouts["Loadouts"].([]any)
	for _, p := range players {
		player := p.(map[string]any)
		subject := player["Subject"].(string)
		characterID := player["CharacterID"].(string)

		loadout := player["Loadout"].(map[string]any)
		items := loadout["Items"].(map[string]any)

		var skins []string
		weaponSkins := map[string]SkinInfo{}
		for weaponID, i := range items {
			item := i.(map[string]any)
			sockets := item["Sockets"].(map[string]any)

			skinSocket, ok := sockets[skinSocketID].(map[string]any)
			if !ok {
				continue
			}
			skinItem := skinSocket["Item"].(map[string]any)
			skinID := skinItem["ID"].(string)

			if info, ok := skinIndex[skinID]; ok {
				skins = append(skins, info.Name)
				weaponSkins[weaponID] = info
			}
		}

		var primarySkin string
		if item, ok := items[primaryWeaponID].(map[string]any); ok {
			if sockets, ok := item["Sockets"].(map[string]any); ok {
				if skinSocket, ok := sockets[skinSocketID].(map[string]any); ok {
					if skinItem, ok := skinSocket["Item"].(map[string]any); ok {
						if skinID, ok := skinItem["ID"].(string); ok {
							primarySkin = skinIndex[skinID].Name
						}
					}
				}
			}
		}
		summaries = append(summaries, PlayerLoadout{
			Subject:     subject,
			AgentName:   agentIndex[characterID],
			Skins:       skins,
			PrimarySkin: primarySkin,
			WeaponSkins: weaponSkins,
		})
	}

	return summaries
}

// teamsFromMatch maps puuid to TeamID. Missing fields are skipped.
func teamsFromMatch(match map[string]any) map[string]string {
	teams := map[string]string{}

	players, ok := match["Players"].([]any)
	if !ok {
		return teams
	}
	for _, p := range players {
		player, ok := p.(map[string]any)
		if !ok {
			continue
		}
		subject, ok := player["Subject"].(string)
		if !ok {
			continue
		}
		if teamID, ok := player["TeamID"].(string); ok {
			teams[subject] = teamID
		}
	}

	return teams
}
