package main

// MatchStats is the HS% and win rate over recent matches.
type MatchStats struct {
	HSPercent float64
	WinRate   float64
	Matches   int
}

// computeMatchStats gets HS% and win rate for puuid from match details.
// Field names are from community docs, so missing fields are skipped instead of panicking.
func computeMatchStats(matchDetailsList []map[string]any, puuid string) MatchStats {
	var totalHead, totalBody, totalLeg float64
	var wins, counted int

	for _, match := range matchDetailsList {
		played := false

		if rounds, ok := match["roundResults"].([]any); ok {
			for _, r := range rounds {
				round, ok := r.(map[string]any)
				if !ok {
					continue
				}
				playerStats, ok := round["playerStats"].([]any)
				if !ok {
					continue
				}
				for _, s := range playerStats {
					ps, ok := s.(map[string]any)
					if !ok {
						continue
					}
					if subject, _ := ps["subject"].(string); subject != puuid {
						continue
					}
					played = true

					damages, ok := ps["damage"].([]any)
					if !ok {
						continue
					}
					for _, d := range damages {
						dm, ok := d.(map[string]any)
						if !ok {
							continue
						}
						if v, ok := dm["headshots"].(float64); ok {
							totalHead += v
						}
						if v, ok := dm["bodyshots"].(float64); ok {
							totalBody += v
						}
						if v, ok := dm["legshots"].(float64); ok {
							totalLeg += v
						}
					}
				}
			}
		}

		if !played {
			continue
		}
		counted++

		if won := didPlayerWin(match, puuid); won {
			wins++
		}
	}

	stats := MatchStats{Matches: counted}
	if total := totalHead + totalBody + totalLeg; total > 0 {
		stats.HSPercent = totalHead / total * 100
	}
	if counted > 0 {
		stats.WinRate = float64(wins) / float64(counted) * 100
	}

	return stats
}

func didPlayerWin(match map[string]any, puuid string) bool {
	players, ok := match["players"].([]any)
	if !ok {
		return false
	}

	var teamID string
	for _, p := range players {
		pl, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if subject, _ := pl["subject"].(string); subject == puuid {
			teamID, _ = pl["teamId"].(string)
			break
		}
	}
	if teamID == "" {
		return false
	}

	teams, ok := match["teams"].([]any)
	if !ok {
		return false
	}
	for _, t := range teams {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := tm["teamId"].(string); id != teamID {
			continue
		}
		won, _ := tm["won"].(bool)
		return won
	}

	return false
}
