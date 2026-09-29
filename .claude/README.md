# riotrpc — project guide

A terminal app that detects whether League of Legends or Valorant is running (both launch
through the Riot Client) and shows live game info via Discord Rich Presence and a Bubble Tea
terminal UI.


## How this project runs

This is a learning project. The person writes the code; Claude is the guide.

- Claude explains concepts, proposes a function/struct signature, and reviews what gets written.
- The person writes the actual logic — especially anything that's a real design decision
  (data structures, error handling, control flow), not just typing what's dictated.
- Claude handles routine scaffolding only: `go.mod`/directory setup, boilerplate that has one
  obvious shape, and glue code that isn't itself a learning opportunity.
- Goal is to learn Go idioms and good software practice generally, not just to finish the app.

## Constraints

- **Read-only, always.** Only call the local/regional Riot APIs Riot already exposes. Never
  read process memory, inject code, or otherwise touch the game client. This is what keeps the
  project distinct from a cheat — see the Notion doc's Safety notes section.
- No upfront package-splitting. Start flat (`main.go`), split into packages only when a second
  concern (e.g. adding Valorant after League, or adding the TUI after the API clients) actually
  forces it. Avoid designing a `internal/league`, `internal/valorant`, `internal/tui` tree before
  there's a reason for each piece.
- Self-signed local certs: League's and Valorant's local HTTPS endpoints use self-signed
  certificates. Use a dedicated `http.Client` with `InsecureSkipVerify` scoped to `127.0.0.1`
  calls only — never set that on `http.DefaultClient`, since the remote PD/GLZ calls must verify
  TLS normally.
- Credentials (lockfile password, tokens) are fine to print while debugging locally, but must
  never be logged persistently or committed.

## Build order

Reordered from the Notion doc so every milestone produces something runnable against real data
quickly, and so shared code (the lockfile parser) gets written once. Currently building
**Valorant first** (the person's choice)

- [x] **M0** — `go mod init`, hello world, confirm `go run .` works
- [x] **M1** — Parse the Riot lockfile (`name:pid:port:password:protocol`). Shared between League
      and Valorant — same format, same code. `Lockfile` struct + `parseLockfile` in `lockfile.go`.
- [x] **M2** — Valorant local API: basic auth (`riot:<password>`) against the lockfile port,
      fetch entitlement + access tokens. `EntitlementsToken` + `fetchEntitlements` in `valorant.go`.
- [x] **M3** — Valorant PD/GLZ token exchange for match state, MMR, inventory. `fetchRegionLocale`,
      `regionToShard`/`shardFromRegion`, `fetchClientVersion` (pulls current patch from the public
      valorant-api.com, no local Valorant install needed), and `fetchMMR` (sets Authorization Bearer,
      X-Riot-Entitlements-JWT, X-Riot-ClientPlatform, X-Riot-ClientVersion) all in `valorant.go`,
      wired in `main.go`. Confirmed working end to end with real MMR/rank data returned. Learned
      along the way: PUUID/entitlements/match data are account-bound and server-side, so this
      works even on a machine with only League installed, as long as the account has played
      Valorant before.
- [x] **M4** — Valorant GLZ live data. Reordered ahead of Discord RPC and the TUI, per the
      person's call — this is the actual motivating use case (seeing teammates'/enemies' skins
      and stats without alt-tabbing to a website). Confirmed working end to end, live, against a
      real custom match. All in `valorant.go`:
        - `fetchPartyID` (`/parties/v1/players/<puuid>`) + `fetchParty`
          (`/parties/v1/parties/<partyID>`) — full roster: members, `PlatformType`, ping data,
          `PlayerIdentity` (account level, card/title IDs). No weapon skins here (cosmetic
          profile data only).
        - `fetchPregameMatchID` + `fetchPregameMatch` (`pregame/v1/players/<puuid>` →
          `pregame/v1/matches/<matchID>`) — agent-select-phase state (`PregameState`, team
          rosters). Only exists during agent select; 404s once the round starts.
        - `fetchCoreGameMatchID` + `fetchLoadouts`
          (`core-game/v1/players/<puuid>` → `core-game/v1/matches/<matchID>/loadouts`) —
          **the actual goal**: every player's equipped skins, per weapon, as raw UUIDs
          (`TypeID`/`ID` pairs under `Loadout.Items[...].Sockets`).
      Key findings along the way:
        - Client version for GLZ has to come from `readClientVersionFromLog()` (parses
          `CI server version:` out of `%LOCALAPPDATA%\VALORANT\Saved\Logs\ShooterGame.log`) —
          the public valorant-api.com version caused `409 CLIENT_VERSION_MISTMATCH`. PD
          (`fetchMMR`) tolerates the public version fine; GLZ does not.
        - GLZ tracks *live session* state — unlike PD (account-bound, works anytime), these
          calls only return real data while Valorant is actually running and logged in
          (confirmed `404 PLAYER_DOES_NOT_EXIST` when it wasn't, and per-phase 404s are normal
          — e.g. pregame 404s once you're in the actual round).
        - `fetchWeaponSkins`/`buildSkinNameIndex` + `fetchAgents`/`buildAgentNameIndex`
          (`valorantdata.go`, public/unauthenticated `valorant-api.com` static data) +
          `summarizeLoadouts` (`valorantdata.go`) — resolve the raw loadout data into a
          `[]PlayerLoadout` (puuid, agent name, equipped skin names). Confirmed live: real
          output is e.g. `868886a3-... playing Jett, skins: [Jigsaw Ares Standard Outlaw ...]`.
          Key gotcha found here: a loadout's equipped-skin socket (`bcef87d6-209b-46c6-8b19-
          fbe40bd95abc`, see `skinSocketID` const) references a skin's own base `uuid` field,
          not a nested `levels[].uuid` — verified empirically since the API docs don't spell
          this out. `buildSkinNameIndex` indexes both to be safe.
      Not yet resolved (left as raw UUIDs, future work): the other weapon item sockets
      (buddy charm, spray, chroma variant selection — only the skin socket is decoded), and
      this has only been tested with one real player in a bot/custom lobby, not a full 10-player
      match with actual teammates and enemies.
- [~] **M5** — Bubble Tea terminal UI to render the M4 data well (the actual reason for wanting
      a TUI over vRY's approach: resizing-safe layout, no browser required for skins/stats).
      In progress, in `tui.go`:
        - `model`/`Init`/`Update`/`View` scaffolded and confirmed rendering + quitting (`q`)
          correctly. `model.cursor` is a placeholder counter (`up`/`down`/`k`/`j`) proving the
          Update -> View loop works, not yet tied to a real list.
        - First real data wired in: `fetchMMRCmd() tea.Msg` reruns the full lockfile ->
          entitlements -> region -> shard -> `fetchMMR` chain (same one M3 validated in the old
          `main.go`, which is now fully commented out in favor of `runTUI()`) as a `tea.Cmd`
          returned from `Init()`. Result comes back as a `mmrMsg{Data, Err}` handled in a
          `case mmrMsg:` branch in `Update`, stored on `model.mmr`/`model.err`. Confirmed live:
          `View()` extracts and displays `RankedRatingAfterUpdate` and `CompetitiveMovement`
          from `mmr["LatestCompetitiveUpdate"]`.
        - Party and loadouts wired in with the same `Cmd`/`msg` pattern: `fetchPartyCmd` and
          `fetchLoadoutsCmd` (reusing `fetchWeaponSkins`/`buildSkinNameIndex`/`fetchAgents`/
          `buildAgentNameIndex`/`summarizeLoadouts` from M4), both run concurrently with
          `fetchMMRCmd` via `tea.Batch(...)` in `Init()`. Confirmed live: real rank, party
          roster, and equipped skins all render together in one terminal screen — the actual
          motivating use case for this whole project.
        - Shared auth chain (lockfile -> entitlements -> region -> shard) extracted into a
          single `authenticate()` helper once three separate `fetchXCmd` functions were about
          to duplicate it a third time.
        - Explicit loading/error/loaded states per section (via a `switch` in `View()`, not
          nested `if`/`else`), instead of showing blank output before data arrives.
        - Party member puuids resolved to real Riot IDs (`Name#Tag`) via a new
          `fetchPlayerNames` (PD `name-service/v2/players`, PUT with a puuid-array body) in
          `valorant.go`. Triggered as a *chained* command: `Update`'s `case partyMsg:` branch
          extracts puuids from the party response and returns `fetchNamesCmd(puuids)` — the
          first example here of one fetch depending on another's result rather than everything
          firing from `Init()` up front.
        - Lip Gloss styling added: `headerStyle`/`errorStyle`/`boxStyle` package-level vars,
          bold colored section headers, red error text, whole output wrapped in a rounded
          border box. Confirmed live and looks good.
        - Two-tab structure added: `model.activeTab` (0/1), toggled with the `tab` key,
          `View()` dispatches to `summaryView()` (rank + party) or `playerSelectView()`
          (loadouts). `model.cursor` finally does real work here — index into `m.loadouts`,
          clamped to valid range, `up`/`down` move it, and only the currently-selected
          player's skin list expands underneath their name. Confirmed live and working.
      Not yet done:
        - Tabs are currently keyboard-only (`tab` to switch). Person wants real clickable tabs
          eventually — doable via `tea.MouseMsg` (gives click coordinates) plus manual
          hit-testing against each tab's rendered region, since Bubbles has no built-in
          clickable tab-bar component. Deferred until keyboard nav is solid.
        - Planned navigation restructure (per vRY reference screenshots): the current
          "loadouts" screen should actually be ONE view that auto-switches between three game
          states — In-Menus, Agent Select, In-Game — based on which GLZ endpoint currently
          succeeds (party-only = in menus, pregame succeeds = agent select, core-game succeeds
          = in-game; same 404-per-phase behavior already discovered in M4). This is NOT a
          user-navigated tab — it should update on its own via periodic re-polling
          (`tea.Tick`), unlike the two real user tabs planned alongside it: a Skins tab
          (browse/pick your own loadout skins, unrelated to the live match view) and a Settings
          tab (e.g. picking which weapon shows as the "primary" column instead of hardcoded
          Vandal). Not started — current `playerSelectView` only handles the in-game case.
        - Layout doesn't resize gracefully with the terminal window (the exact vRY complaint
          that motivated switching to Bubble Tea in the first place — still unresolved, just
          deferred past the initial styling pass).
        - Per-player grouping is now solved for the loadouts tab specifically (see above), but
          the summary tab still shows rank/party as separate global sections rather than a
          true vRY-style per-player card grid. May want `lipgloss.JoinHorizontal`/
          `JoinVertical` here too eventually.
        - The type assertions in `View()`/`fetchXCmd` are unchecked (`.(float64)`/
          `.(map[string]any)` without comma-ok) — fine for now since these fields are confirmed
          present in a real response, but will panic if e.g. `LatestCompetitiveUpdate` is ever
          nil (no ranked match played) — a concern to actually resolve as part of M9.
        - Team colors: ALLIES and ENEMIES headers (and ideally their rows, on both the Match
          and Skins tabs) should get distinct colors — e.g. Valorant's own teal/cyan for
          allies and red for enemies. Both currently share `weaponCategoryStyle` (purple).
          Likely approach: a style/color field on `rosterSection`.
        - Valorant-themed overall look: replace the current purple/pink Lip Gloss palette
          (`boxStyle`, tab styles, `headerStyle`, status bar chips) with Valorant's colors —
          its signature red (#FF4655), off-white (#ECE8E1), and dark navy (#0F1923). Worth
          collecting the colors into one set of named palette vars first, so the theme is
          changed in one place instead of across every style.
- [ ] **M6** — Discord Rich Presence, using whichever data (Valorant + eventually League) is
      available by then
- [ ] **M7** — League LCU auth + profile/rank (reuses the M1 lockfile parser)
- [ ] **M8** — League Live Client Data API (port 2999, in-match only — test via Practice Tool)
- [ ] **M9** — Graceful handling when neither game is running

## Key references

- `lcu-gopher` — Go LCU driver
- valorant-api.com — static Valorant game data
- Bubble Tea / Bubbles / Lip Gloss — planned TUI stack
- Discord RPC Go module — not yet chosen; verify current options on pkg.go.dev before M4