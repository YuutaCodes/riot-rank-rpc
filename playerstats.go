package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// PlayerStats is one player's rank and recent performance, for the in-game table.
type PlayerStats struct {
	Tier     int
	RR       float64
	DeltaRR  float64 // RR earned in the last competitive game.
	PeakTier int
	Stats    MatchStats
	Err      error
}

type playerStatsMsg struct {
	Subject string
	Data    PlayerStats
}

// fetchPlayerStatsCmd loads rank, peak rank, HS% and win rate for one player.
// That's one MMR call plus the match history and up to 5 match details.
func fetchPlayerStatsCmd(subject string) tea.Cmd {
	return func() tea.Msg {
		entitlements, shard, err := authenticate()
		if err != nil {
			return playerStatsMsg{Subject: subject, Data: PlayerStats{Err: err}}
		}

		var ps PlayerStats
		mmr, err := fetchMMR(shard, subject, entitlements.AccessToken, entitlements.Token)
		if err != nil {
			return playerStatsMsg{Subject: subject, Data: PlayerStats{Err: err}}
		}
		if update, ok := mmr["LatestCompetitiveUpdate"].(map[string]any); ok {
			tier, _ := update["TierAfterUpdate"].(float64)
			ps.Tier = int(tier)
			ps.RR, _ = update["RankedRatingAfterUpdate"].(float64)
			ps.DeltaRR, _ = update["RankedRatingEarned"].(float64)
		}
		ps.PeakTier = max(ps.Tier, peakTierFromMMR(mmr))

		// Stats are optional, so a failed history fetch still shows the rank.
		history, err := fetchMatchHistory(shard, subject, entitlements.AccessToken, entitlements.Token)
		if err != nil {
			return playerStatsMsg{Subject: subject, Data: ps}
		}
		entries, _ := history["History"].([]any)
		var details []map[string]any
		for _, e := range entries {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			matchID, ok := entry["MatchID"].(string)
			if !ok {
				continue
			}
			if match, err := fetchMatchDetails(shard, matchID, entitlements.AccessToken, entitlements.Token); err == nil {
				details = append(details, match)
			}
		}
		ps.Stats = computeMatchStats(details, subject)

		return playerStatsMsg{Subject: subject, Data: ps}
	}
}

// rosterRow is one player in a Match tab table. Agent and Skin are empty when the
// phase doesn't have them (no agent in the lobby, no skins before the match starts).
type rosterRow struct {
	Subject   string
	Agent     string
	Level     string
	Skin      string
	Incognito bool // Streamer mode is on, so identity and stats stay hidden.
}

// isIncognito reports whether a PlayerIdentity has streamer mode on.
func isIncognito(identity map[string]any) bool {
	hidden, _ := identity["Incognito"].(bool)
	return hidden
}

// incognitoFromMatch maps each player's puuid to whether they're in streamer mode,
// from a core-game match response.
func incognitoFromMatch(match map[string]any) map[string]bool {
	incognito := map[string]bool{}
	players, _ := match["Players"].([]any)
	for _, p := range players {
		player, _ := p.(map[string]any)
		subject, _ := player["Subject"].(string)
		identity, _ := player["PlayerIdentity"].(map[string]any)
		if subject != "" && isIncognito(identity) {
			incognito[subject] = true
		}
	}
	return incognito
}

// levelFromIdentity reads the account level from a PlayerIdentity object.
// It returns "-" if it's missing or the player hides it.
func levelFromIdentity(identity map[string]any) string {
	if hidden, _ := identity["HideAccountLevel"].(bool); hidden {
		return "-"
	}
	if level, ok := identity["AccountLevel"].(float64); ok {
		return fmt.Sprintf("%.0f", level)
	}
	return "-"
}

// levelsFromMatch maps each player's puuid to their account level from a core-game
// match response.
func levelsFromMatch(match map[string]any) map[string]string {
	levels := map[string]string{}

	players, _ := match["Players"].([]any)
	for _, p := range players {
		player, _ := p.(map[string]any)
		subject, _ := player["Subject"].(string)
		identity, _ := player["PlayerIdentity"].(map[string]any)
		if subject != "" {
			levels[subject] = levelFromIdentity(identity)
		}
	}

	return levels
}

// partyRows lists party members from a party response.
func partyRows(party map[string]any) []rosterRow {
	var rows []rosterRow
	members, _ := party["Members"].([]any)
	for _, mem := range members {
		member, _ := mem.(map[string]any)
		subject, _ := member["Subject"].(string)
		if subject == "" {
			continue
		}
		identity, _ := member["PlayerIdentity"].(map[string]any)
		rows = append(rows, rosterRow{Subject: subject, Level: levelFromIdentity(identity)})
	}
	return rows
}

// pregameRows lists your team from a pregame match response. Only your own team is
// visible during agent select. The agent is blank until the player hovers or locks one.
func pregameRows(match map[string]any, agentIndex map[string]string) []rosterRow {
	var rows []rosterRow
	team, _ := match["AllyTeam"].(map[string]any)
	players, _ := team["Players"].([]any)
	for _, p := range players {
		player, _ := p.(map[string]any)
		subject, _ := player["Subject"].(string)
		if subject == "" {
			continue
		}
		characterID, _ := player["CharacterID"].(string)
		agent := agentIndex[strings.ToLower(characterID)]
		if agent == "" {
			agent = "-"
		} else if state, _ := player["CharacterSelectionState"].(string); state != "locked" {
			agent += "?"
		}
		identity, _ := player["PlayerIdentity"].(map[string]any)
		rows = append(rows, rosterRow{Subject: subject, Agent: agent, Level: levelFromIdentity(identity), Incognito: isIncognito(identity)})
	}
	return rows
}
