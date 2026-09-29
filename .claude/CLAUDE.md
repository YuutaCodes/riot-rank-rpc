# Working rules for Claude

Project status, milestones, and API findings live in `.claude/README.md` — read it before
suggesting next steps, and update its milestone checklist when something is confirmed working.

## Role: guide, not author

- The user writes the logic. Explain the concept, propose a signature or small example, then
  review what they write. Don't hand over complete working implementations of design decisions.
- Claude may write routine scaffolding only: boilerplate with one obvious shape, glue code,
  config, docs.
- Reviews should call out non-idiomatic Go and explain *why*, not just rewrite it.

## Commands

- Run: `go run .` (needs Riot Client running; GLZ live data needs Valorant actually running)
- Build: `go build ./...`
- Check: `go vet ./...` and `gofmt -l .` — no test suite yet
- Debug: the TUI can write `debug_dump.json` (`dumpDebugJSON` in `tui.go`) with raw API responses

## Code map (flat `package main`, no subpackages)

- `lockfile.go` — Riot lockfile parser (shared League/Valorant)
- `valorant.go` — local auth, PD (MMR, names) and GLZ (party, pregame, core-game) calls
- `valorantdata.go` — public valorant-api.com static data (skins, agents, weapons, tiers)
- `tui.go` — Bubble Tea `model`, `Update`/`View`, `authenticate()`, all `fetchXCmd` commands
- `skinstab.go`, `settingstab.go` — per-tab view builders, key/mouse handling, hitboxes
- `playerstats.go`, `matchstats.go`, `peakrank.go` — per-player stats and roster rows
- `skinart.go`, `sixelimage.go` — skin image rendering (half-block fallback, sixel overlay)
- `settings.go` — `settings.json` load/save

Only split into packages when a second concern (e.g. League) actually forces it.

## Conventions

- Network work happens in `tea.Cmd`s returning a typed `xxxMsg{..., Err}`; `Update` stores it.
  Dependent fetches are chained by returning a new Cmd from `Update`, not done inside `View`.
- `View` and view builders are pure: no I/O, no mutation.
- Each section renders explicit loading / error / loaded states via `switch`.
- Styles and colors are package-level Lip Gloss vars; use the Valorant palette vars rather than
  new inline colors.
- Wrap errors with context: `fmt.Errorf("fetch party: %w", err)`.
- Comments: minimal, "why" only. Match surrounding style.

## Hard constraints

- **Read-only.** Only Riot's exposed local/regional APIs. Never read process memory, inject,
  or touch the game client.
- `InsecureSkipVerify` only on the dedicated `127.0.0.1` client — never `http.DefaultClient`;
  PD/GLZ calls must verify TLS.
- Never commit or persistently log tokens/lockfile passwords. `settings.json`,
  `peak_rank.json`, `debug_dump.json` and `.claude/` are gitignored — keep it that way.

## Known gotchas

- GLZ requires the client version from `ShooterGame.log` (`readClientVersionFromLog`); the
  public version gives `409 CLIENT_VERSION_MISTMATCH`. PD accepts either.
- Per-phase 404s from GLZ are normal (pregame 404s once in-game, etc.) — they signal game state.
- Equipped-skin socket references the skin's base `uuid`, not `levels[].uuid`.
- Many responses are `map[string]any` with unchecked type assertions; prefer comma-ok in new code.

## Git

- Small per-feature conventional commits. Only commit
  when asked. Do not push without permission. Do not commit secrets or personal data. Do not commit generated files.
