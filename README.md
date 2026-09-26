# Riot Rank RPC (WIP)

Project to learn Golang, Bubble Tea, and personal interest.

A terminal app that detects whether League of Legends or Valorant is running (both launch
through the Riot Client) and shows live game info - rank, party, live match state, and equipped
skins - in a terminal UI, with Discord Rich Presence planned.

Currently Valorant-first. Reads only Riot's own local/regional APIs

## Status

- [x] Lockfile parsing (shared between League/Valorant)
- [x] Valorant local auth (entitlements + access tokens)
- [x] Valorant MMR/rank data
- [x] Valorant live match data (party, pregame, core-game, equipped skins)
- [~] Terminal UI (Bubble Tea) — in progress
- [ ] Discord Rich Presence
- [ ] League of Legends support

## Requirements

- Go 1.27+
- Riot Client running (League and/or Valorant)

## Running

```
go run .
```