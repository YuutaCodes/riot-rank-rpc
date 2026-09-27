package main

import (
	"encoding/json"
	"os"
	"strconv"
)

const peakRankFile = "peak_rank.json"

// loadPeakRank reads the saved peak tier per puuid. Riot's MMR response has no peak field,
// so we store it ourselves.
func loadPeakRank() map[string]int {
	data, err := os.ReadFile(peakRankFile)
	if err != nil {
		return map[string]int{}
	}

	var peaks map[string]int
	if err := json.Unmarshal(data, &peaks); err != nil {
		return map[string]int{}
	}

	return peaks
}

func savePeakRank(peaks map[string]int) error {
	data, err := json.MarshalIndent(peaks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(peakRankFile, data, 0644)
}

// peakTierFromMMR returns the highest tier won at in any competitive season.
// WinsByTier is keyed by tier number, like {"14": 10, "15": 3}.
func peakTierFromMMR(mmr map[string]any) int {
	queues, _ := mmr["QueueSkills"].(map[string]any)
	competitive, _ := queues["competitive"].(map[string]any)
	seasons, _ := competitive["SeasonalInfoBySeasonID"].(map[string]any)

	peak := 0
	for _, s := range seasons {
		season, _ := s.(map[string]any)
		wins, _ := season["WinsByTier"].(map[string]any)
		for key := range wins {
			if tier, err := strconv.Atoi(key); err == nil && tier > peak {
				peak = tier
			}
		}
	}
	return peak
}

// updatePeakRank saves tier if it beats the current peak and returns the peak.
func updatePeakRank(subject string, tier int) int {
	peaks := loadPeakRank()

	if tier > peaks[subject] {
		peaks[subject] = tier
		_ = savePeakRank(peaks)
	}

	return peaks[subject]
}
