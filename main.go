package main

import (
	"fmt"
	"os"
)

func main() {
	path := os.Getenv("LOCALAPPDATA") + `\Riot Games\Riot Client\Config\lockfile`

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("could not read lockfile (is the Riot Client running?):", err)
		return
	}

	lf, err := parseLockfile(string(data))
	if err != nil {
		fmt.Println("failed to parse lockfile:", err)
		return
	}

	entitlements, err := fetchEntitlements(lf)
	if err != nil {
		fmt.Println("failed to fetch entitlements:", err)
		return
	}

	regionLocale, err := fetchRegionLocale(lf)
	if err != nil {
		fmt.Println("failed to fetch region locale:", err)
		return
	}

	// KNOWN ISSUE: regionLocale.Region comes back as a League-style code (e.g. "EUW"),
	// not a real Valorant shard ("eu", "na", "ap", "kr"), so this currently fails with
	// "no such host" on a machine that only has League installed. Needs testing/fixing
	// on a machine with Valorant installed to find the real Valorant region-locale shape.
	mmr, err := fetchMMR(shardFromRegion(regionLocale.Region), entitlements.Subject, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		fmt.Println("failed to fetch MMR:", err)
		return
	}

	partyID, err := fetchPartyID(shardFromRegion(regionLocale.Region), entitlements.Subject, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		fmt.Println("failed to fetch party ID:", err)
		return
	}

	party, err := fetchParty(shardFromRegion(regionLocale.Region), partyID, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		fmt.Println("failed to fetch party:", err)
		return
	}

	pregameMatchID, err := fetchPregameMatchID(shardFromRegion(regionLocale.Region), entitlements.Subject, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		fmt.Println("failed to fetch pregame match ID:", err)
		return
	}

	pregameMatch, err := fetchPregameMatch(shardFromRegion(regionLocale.Region), pregameMatchID.MatchID, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		fmt.Println("failed to fetch pregame match:", err)
		return
	}

	coreGameMatchID, err := fetchCoreGameMatchID(shardFromRegion(regionLocale.Region), entitlements.Subject, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		fmt.Println("failed to fetch core-game match ID:", err)
		return
	}

	loadouts, err := fetchLoadouts(shardFromRegion(regionLocale.Region), coreGameMatchID.MatchID, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		fmt.Println("failed to fetch loadouts:", err)
		return
	}

	fmt.Printf("%+v\n", lf)
	fmt.Printf("%+v\n", entitlements)
	fmt.Printf("%+v\n", regionLocale)
	fmt.Printf("%+v\n", mmr)
	fmt.Printf("%+v\n", partyID)
	fmt.Printf("%+v\n", party)
	fmt.Println()
	fmt.Printf("%+v\n", pregameMatchID)
	fmt.Println()
	fmt.Printf("%+v\n", pregameMatch)
	fmt.Println()
	fmt.Printf("%+v\n", coreGameMatchID)
	fmt.Println()
	fmt.Printf("%+v\n", loadouts)
}
