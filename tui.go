package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	cursor int
	mmr    map[string]any
	err    error
}

type mmrMsg struct {
	Data map[string]any
	Err  error
}

func fetchMMRCmd() tea.Msg {
	path := os.Getenv("LOCALAPPDATA") + `\Riot Games\Riot Client\Config\lockfile`
	data, err := os.ReadFile(path)

	if err != nil {
		return mmrMsg{Err: err}
	}

	lf, err := parseLockfile(string(data))
	if err != nil {
		return mmrMsg{Err: err}
	}

	entitlements, err := fetchEntitlements(lf)
	if err != nil {
		return mmrMsg{Err: err}
	}

	regionLocale, err := fetchRegionLocale(lf)
	if err != nil {
		return mmrMsg{Err: err}
	}
	mmr, err := fetchMMR(shardFromRegion(regionLocale.Region), entitlements.Subject, entitlements.AccessToken, entitlements.Token)
	if err != nil {
		return mmrMsg{Err: err}
	}

	return mmrMsg{Data: mmr}
}

func (m model) Init() tea.Cmd {
	return fetchMMRCmd
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
	errLine := ""
	if m.err != nil {
		errLine = fmt.Sprintf("Error: %v\n", m.err)
	}
	return cursorLine + mmrLine + errLine
}

func runTUI() {
	p := tea.NewProgram(model{})
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
