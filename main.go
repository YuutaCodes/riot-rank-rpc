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

	fmt.Printf("%+v\n", lf)
	fmt.Printf("%+v\n", entitlements)
	fmt.Printf("%+v\n", regionLocale)
	fmt.Printf("%+v\n", mmr)
}
