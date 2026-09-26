package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	cursor   int
	mmr      map[string]any
	err      error
	party    map[string]any
	loadouts []PlayerLoadout
}

type mmrMsg struct {
	Data map[string]any
	Err  error
}

type partyMsg struct {
	Data map[string]any
	Err  error
}

type loadoutsMsg struct {
	Data []PlayerLoadout
	Err  error
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchMMRCmd, fetchPartyCmd, fetchLoadoutsCmd)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			m.cursor++
		}

	case mmrMsg:
		m.mmr = msg.Data
		m.err = msg.Err
	case partyMsg:
		m.party = msg.Data
		m.err = msg.Err
	case loadoutsMsg:
		m.loadouts = msg.Data
		m.err = msg.Err
	}
	return m, nil
}

func (m model) View() string {
	cursorLine := fmt.Sprintf("Cursor position: %d\nPress q to quit.\n", m.cursor)
	mmrLine := ""
	if m.mmr != nil {
		update := m.mmr["LatestCompetitiveUpdate"].(map[string]any)
		rr := update["RankedRatingAfterUpdate"].(float64)
		movement := update["CompetitiveMovement"].(string)
		mmrLine = fmt.Sprintf("Ranked Rating: %.0f (%s)\n", rr, movement)
	}
	partyLine := ""
	if m.party != nil {
		members := m.party["Members"].([]any)
		partyLine = fmt.Sprintf("Party size: %d\n", len(members))
		for _, mem := range members {
			member := mem.(map[string]any)
			partyLine += fmt.Sprintf("  %s\n", member["Subject"].(string))
		}
	}
	loadoutsLines := ""
	for _, l := range m.loadouts {
		loadoutsLines += fmt.Sprintf("%s playing %s, skins: %v\n", l.Subject, l.AgentName, l.Skins)
	}
	errLine := ""
	if m.err != nil {
		errLine = fmt.Sprintf("Error: %v\n", m.err)
	}
	return cursorLine + mmrLine + partyLine + loadoutsLines + errLine
}

func runTUI() {
	p := tea.NewProgram(model{})
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

func authenticate() (EntitlementsToken, string, error) {
	path := os.Getenv("LOCALAPPDATA") + `\Riot Games\Riot Client\Config\lockfile`
	data, err := os.ReadFile(path)

	if err != nil {
		return EntitlementsToken{}, "", fmt.Errorf("could not read lockfile (is the Riot Client running?): %w", err)
	}

	lf, err := parseLockfile(string(data))
	if err != nil {
		return EntitlementsToken{}, "", fmt.Errorf("failed to parse lockfile: %w", err)
	}

	entitlements, err := fetchEntitlements(lf)
	if err != nil {
		return EntitlementsToken{}, "", fmt.Errorf("failed to fetch entitlements: %w", err)
	}

	regionLocale, err := fetchRegionLocale(lf)
	if err != nil {
		return EntitlementsToken{}, "", fmt.Errorf("failed to fetch region locale: %w", err)
	}
	return entitlements, shardFromRegion(regionLocale.Region), nil
}

func fetchMMRCmd() tea.Msg {
	entitlements, shard, err := authenticate()
	if err != nil {
		return mmrMsg{Err: err}
	}

	mmr, err := fetchMMR(shard, entitlements.Subject, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		return mmrMsg{Err: err}
	}

	return mmrMsg{Data: mmr}
}

func fetchPartyCmd() tea.Msg {
	entitlements, shard, err := authenticate()
	if err != nil {
		return partyMsg{Err: err}
	}

	partyID, err := fetchPartyID(shard, entitlements.Subject, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		return partyMsg{Err: err}
	}

	party, err := fetchParty(shard, partyID, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		return partyMsg{Err: err}
	}

	return partyMsg{Data: party}
}

func fetchLoadoutsCmd() tea.Msg {
	entitlements, shard, err := authenticate()
	if err != nil {
		return loadoutsMsg{Err: err}
	}

	matchID, err := fetchCoreGameMatchID(shard, entitlements.Subject, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		return loadoutsMsg{Err: err}
	}

	loadouts, err := fetchLoadouts(shard, matchID.MatchID, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		return loadoutsMsg{Err: err}
	}

	skins, err := fetchWeaponSkins()
	if err != nil {
		return loadoutsMsg{Err: err}
	}
	skinIndex := buildSkinNameIndex(skins["data"].([]any))

	agents, err := fetchAgents()
	if err != nil {
		return loadoutsMsg{Err: err}
	}
	agentIndex := buildAgentNameIndex(agents["data"].([]any))

	summaries := summarizeLoadouts(loadouts, skinIndex, agentIndex)

	return loadoutsMsg{Data: summaries}
}
