package main

import (
	"strconv"
	"strings"
)

type Lockfile struct {
	Name     string
	Pid      int
	Port     int
	Password string
	Protocol string
}

// Get the lockfile from the Riot Client
func parseLockfile(raw string) (Lockfile, error) {

	parts := strings.Split(strings.TrimSpace(raw), ":")
	name := parts[0]
	pid, err := strconv.Atoi(parts[1])
	if err != nil {
		return Lockfile{}, err
	}
	port, err := strconv.Atoi(parts[2])
	if err != nil {
		return Lockfile{}, err
	}
	password := parts[3]
	protocol := parts[4]

	return Lockfile{name, pid, port, password, protocol}, nil
}
