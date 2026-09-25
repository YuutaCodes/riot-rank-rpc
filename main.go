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

	fmt.Printf("%+v\n", lf)
	fmt.Printf("%+v\n", entitlements)
}
