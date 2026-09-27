package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	lipgloss "github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"
)

// STRUCTS
type model struct {
	cursor          int
	activeTab       int
	hover           hoverTarget // clickable thing under the mouse, for highlighting
	mmr             map[string]any
	err             error
	party           map[string]any
	loadouts        []PlayerLoadout
	gameState       GameState
	names           map[string]string
	spin            spinner.Model
	width           int
	height          int
	weapons         []WeaponOption
	primaryWeaponID string
	match           matchData
	settingsCursor  int
	shard           string
	mySubject       string
	serverCity      string
	tierNames       map[int]string
	peakTier        int
	playerStats     map[string]PlayerStats // In-game rank and stats, keyed by puuid.
	statsPending    map[string]bool        // Players already requested, so the poll doesn't refetch them.
	pregame         []rosterRow            // Your team during agent select.

	// Skins tab. The player is tracked by puuid because the poll replaces m.loadouts.
	skinsSubject   string
	skinsWeapon    int
	skinsFocus     int
	skinArt        map[string]string
	skinImages     map[string]image.Image // nil means the download failed.
	skinArtPending map[string]bool

	// Settings tab.
	settingsCategory int
	settingsFocus    int
	columnCursor     int
	hiddenColumns    map[string]bool // Match tab columns the user turned off.
}

type skinImageMsg struct {
	URL string
	Img image.Image
	Err error
}

func fetchSkinImageCmd(url string) tea.Cmd {
	return func() tea.Msg {
		img, err := fetchSkinImage(url)
		return skinImageMsg{URL: url, Img: img, Err: err}
	}
}

type mmrMsg struct {
	Data    map[string]any
	Subject string
	Err     error
}

type partyMsg struct {
	Data map[string]any
	Err  error
}

// GameState is the current match phase, based on which GLZ endpoint responds.
type GameState int

const (
	// StateWaiting is the zero value, so a fresh model starts out waiting for the game.
	StateWaiting GameState = iota
	StateMenus
	StateAgentSelect
	StateInGame
)

type gameStateMsg struct {
	State GameState
	Match matchData
	// Pregame is your team during agent select.
	Pregame []rosterRow
	Shard   string
	Err     error
}

// matchData is the raw in-game data. The model keeps it so changing the primary
// weapon can re-summarize locally instead of refetching.
type matchData struct {
	Loadouts   map[string]any
	SkinIndex  map[string]SkinInfo
	AgentIndex map[string]string
	Teams      map[string]string
	Levels     map[string]string
	Incognito  map[string]bool // Players in streamer mode.
}

// summarize builds the per-player loadouts for the given primary weapon.
func (d matchData) summarize(primaryWeaponID string) []PlayerLoadout {
	if d.Loadouts == nil {
		return nil
	}
	summaries := summarizeLoadouts(d.Loadouts, d.SkinIndex, d.AgentIndex, primaryWeaponID)
	for i := range summaries {
		summaries[i].TeamID = d.Teams[summaries[i].Subject]
	}
	return summaries
}

// pollTickMsg triggers a refetch so the view follows phase changes on its own.
type pollTickMsg struct{}

func pollTickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return pollTickMsg{}
	})
}

type namesMsg struct {
	Data map[string]string
	Err  error
}

type weaponsMsg struct {
	Data []WeaponOption
	Err  error
}

func fetchWeaponsCmd() tea.Msg {
	weapons, err := fetchWeapons()
	if err != nil {
		return weaponsMsg{Err: err}
	}
	return weaponsMsg{Data: buildWeaponOptions(weapons["data"].([]any))}
}

type tierNamesMsg struct {
	Data map[int]string
	Err  error
}

func fetchTierNamesCmd() tea.Msg {
	tiers, err := fetchCompetitiveTiers()
	if err != nil {
		return tierNamesMsg{Err: err}
	}
	return tierNamesMsg{Data: buildTierNameIndex(tiers["data"].([]any))}
}

func newModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle
	settings := loadSettings()
	hidden := map[string]bool{}
	for _, c := range settings.HiddenColumns {
		hidden[c] = true
	}
	return model{
		spin:            s,
		primaryWeaponID: settings.PrimaryWeaponID,
		hiddenColumns:   hidden,
		skinArt:         map[string]string{},
		skinImages:      map[string]image.Image{},
		skinArtPending:  map[string]bool{},
		playerStats:     map[string]PlayerStats{},
		statsPending:    map[string]bool{},
	}
}

var spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))

// LIPGLOSS STYLES
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2)

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				Padding(0, 1)

	normalRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("63")).
				Align(lipgloss.Center).
				Padding(0, 1)

	// Folder style tabs. The active tab has an open bottom so it looks raised.
	tabBorder = lipgloss.Border{
		Top: "─", Bottom: "─", Left: "│", Right: "│",
		TopLeft: "╭", TopRight: "╮", BottomLeft: "┴", BottomRight: "┴",
	}

	activeTabBorder = lipgloss.Border{
		Top: "─", Bottom: " ", Left: "│", Right: "│",
		TopLeft: "╭", TopRight: "╮", BottomLeft: "┘", BottomRight: "└",
	}

	inactiveTabStyle = lipgloss.NewStyle().
				Border(tabBorder, true).
				BorderForeground(lipgloss.Color("240")).
				Foreground(lipgloss.Color("240")).
				Padding(0, 1)

	activeTabStyle = inactiveTabStyle.
			Border(activeTabBorder, true).
			BorderForeground(lipgloss.Color("212")).
			Foreground(lipgloss.Color("212")).
			Bold(true)

	// tabGapStyle stretches the line under the tabs to the full width.
	tabGapStyle = inactiveTabStyle.
			BorderTop(false).
			BorderLeft(false).
			BorderRight(false)

	statusBarBackground = lipgloss.NewStyle().Background(lipgloss.Color("235"))

	lobbyStatusStyle       = lipgloss.NewStyle().Background(lipgloss.Color("240")).Foreground(lipgloss.Color("230")).Bold(true).Padding(0, 1)
	agentSelectStatusStyle = lipgloss.NewStyle().Background(lipgloss.Color("214")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1)
	inGameStatusStyle      = lipgloss.NewStyle().Background(lipgloss.Color("205")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1)

	shardChipStyle = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("230")).Bold(true).Padding(0, 3)

	weaponCategoryStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	weaponColumnStyle   = lipgloss.NewStyle().Width(16).MarginBottom(1)

	// hoverBackground marks whatever clickable thing is under the mouse.
	hoverBackground = lipgloss.Color("237")
)

const (
	tabMatch = iota
	tabSkins
	tabSettings
)

// tabLabels are in display order. The index matches model.activeTab.
var tabLabels = []string{"Match", "Skins", "Settings"}

// buildTabBar renders the tab row and returns each tab's column range.
// View and Update both use it, so mouse clicks always match what's drawn.
// hovered is the tab under the mouse, or -1. dimSkins greys out the Skins tab while
// there's no match to show.
func buildTabBar(active, hovered, width int, dimSkins bool) (string, [][2]int) {
	var rendered []string
	var bounds [][2]int
	x := 0

	for i, label := range tabLabels {
		style := inactiveTabStyle
		switch i {
		case active:
			style = activeTabStyle
		case hovered:
			style = style.Foreground(lipgloss.Color("252")).Background(hoverBackground)
		default:
			if dimSkins && i == tabSkins {
				style = style.Foreground(lipgloss.Color("237"))
			}
		}
		cell := style.Render(label)
		cellWidth := lipgloss.Width(cell)
		bounds = append(bounds, [2]int{x, x + cellWidth})
		x += cellWidth

		rendered = append(rendered, cell)
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)

	gapWidth := width - lipgloss.Width(row)
	if gapWidth < 0 {
		gapWidth = 0
	}
	gap := tabGapStyle.Render(strings.Repeat(" ", gapWidth))

	return lipgloss.JoinHorizontal(lipgloss.Bottom, row, gap), bounds
}

// What a hoverTarget points at.
const (
	hoverNone = iota
	hoverTab
	hoverSetting         // index is a weapon in m.weapons
	hoverSkinsPlayer     // subject is the player
	hoverSkinsWeapon     // index is a row in the weapon column
	hoverMatchSkin       // subject is the player
	hoverSettingCategory // index is a settings category
	hoverSettingColumn   // index is in optionalColumns
)

type hoverTarget struct {
	kind    int
	index   int
	subject string
}

// hoverAt returns the clickable thing at screen position x, y. It uses the same
// hitboxes as the click handlers, so hover and click always agree.
func (m model) hoverAt(x, y int) hoverTarget {
	if y < tabBarHeight {
		_, bounds := buildTabBar(m.activeTab, -1, m.width, m.gameState != StateInGame)
		for i, b := range bounds {
			if x >= b[0] && x < b[1] {
				return hoverTarget{kind: hoverTab, index: i}
			}
		}
		return hoverTarget{}
	}

	switch m.activeTab {
	case tabMatch:
		if m.gameState == StateInGame {
			_, hits := m.buildMatchView()
			for _, h := range hits {
				if y == h.y && x >= h.x0 && x < h.x1 {
					return hoverTarget{kind: hoverMatchSkin, subject: h.subject}
				}
			}
		}
	case tabSkins:
		_, hits := m.buildSkins()
		for _, h := range hits {
			if y == h.y && x >= h.x0 && x < h.x1 {
				if h.subject != "" {
					return hoverTarget{kind: hoverSkinsPlayer, subject: h.subject}
				}
				return hoverTarget{kind: hoverSkinsWeapon, index: h.weapon}
			}
		}
	case tabSettings:
		return m.settingsHoverAt(x, y)
	}
	return hoverTarget{}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, fetchMMRCmd, fetchPartyCmd, fetchWeaponsCmd, fetchTierNamesCmd, fetchGameStateCmd(), pollTickCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.skinArt = map[string]string{}
		// Resizing can reflow the old frame, so redraw from scratch instead of diffing.
		return m, tea.Batch(tea.ClearScreen, m.requestSkinArt())

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionMotion {
			m.hover = m.hoverAt(msg.X, msg.Y)
			return m, nil
		}
		if m.activeTab == tabSkins && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
			m.skinsFocus = skinsFocusPlayers
			if msg.X >= settingsContentX+skinsPlayerPaneWidth {
				m.skinsFocus = skinsFocusWeapons
			}
			delta := 1
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -1
			}
			m.moveSkinsCursor(delta)
			return m, m.requestSkinArt()
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y < tabBarHeight {
			_, bounds := buildTabBar(m.activeTab, -1, m.width, m.gameState != StateInGame)
			for i, b := range bounds {
				if msg.X >= b[0] && msg.X < b[1] {
					m.activeTab = i
					break
				}
			}
			if m.activeTab == tabSkins {
				return m, m.requestSkinArt()
			}
		}
		// Clicking a player's skin on the in-game Match table opens them on the Skins tab.
		if m.activeTab == tabMatch && m.gameState == StateInGame && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y >= tabBarHeight {
			_, hits := m.buildMatchView()
			for _, h := range hits {
				if msg.Y == h.y && msg.X >= h.x0 && msg.X < h.x1 {
					for i, s := range m.matchSubjects() {
						if s == h.subject {
							m.cursor = i
						}
					}
					m.activeTab = tabSkins
					m.skinsSubject = h.subject
					m.skinsFocus = skinsFocusWeapons
					m.skinsWeapon = m.primaryWeaponRow()
					return m, m.requestSkinArt()
				}
			}
		}
		if m.activeTab == tabSkins && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y >= tabBarHeight {
			_, hits := m.buildSkins()
			for _, h := range hits {
				if msg.Y == h.y && msg.X >= h.x0 && msg.X < h.x1 {
					if h.subject != "" {
						m.skinsSubject = h.subject
						m.skinsFocus = skinsFocusPlayers
					} else {
						m.skinsWeapon = h.weapon
						m.skinsFocus = skinsFocusWeapons
					}
					return m, m.requestSkinArt()
				}
			}
		}
		if m.activeTab == tabSettings && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y >= tabBarHeight {
			return m.handleSettingsClick(msg.X, msg.Y)
		}

	case tea.KeyMsg:
		if m.activeTab == tabSettings {
			if next, cmd, ok := m.handleSettingsKey(msg.String()); ok {
				return next, cmd
			}
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "d":
			m.err = dumpDebugJSON(m)

		case "up", "k":
			if m.activeTab == tabSkins {
				m.moveSkinsCursor(-1)
				return m, m.requestSkinArt()
			}
			if m.activeTab == tabMatch && m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.activeTab == tabSkins {
				m.moveSkinsCursor(1)
				return m, m.requestSkinArt()
			}
			if m.activeTab == tabMatch && m.cursor < len(m.matchSubjects())-1 {
				m.cursor++
			}

		case "left", "h":
			if m.activeTab == tabSkins {
				m.skinsFocus = skinsFocusPlayers
			}

		case "right", "l":
			if m.activeTab == tabSkins {
				m.skinsFocus = skinsFocusWeapons
			}

		case "tab":
			m.activeTab = (m.activeTab + 1) % len(tabLabels)
			if m.activeTab == tabSkins {
				return m, m.requestSkinArt()
			}
		}

	case mmrMsg:
		m.mmr = msg.Data
		m.mySubject = msg.Subject
		m.err = msg.Err
		if msg.Data != nil {
			update := msg.Data["LatestCompetitiveUpdate"].(map[string]any)
			tier := int(update["TierAfterUpdate"].(float64))
			m.peakTier = updatePeakRank(msg.Subject, max(tier, peakTierFromMMR(msg.Data)))
		}
	case partyMsg:
		m.party = msg.Data
		m.err = msg.Err
		if msg.Data != nil {
			members := msg.Data["Members"].([]any)
			var puuids []string
			for _, mem := range members {
				member := mem.(map[string]any)
				puuids = append(puuids, member["Subject"].(string))
			}

			if matchmakingData, ok := msg.Data["MatchmakingData"].(map[string]any); ok {
				if pods, ok := matchmakingData["PreferredGamePods"].([]any); ok && len(pods) > 0 {
					m.serverCity = cityFromGamePod(pods[0].(string))
				}
			}

			return m, tea.Batch(fetchNamesCmd(puuids), m.queuePlayerData())
		}
	case gameStateMsg:
		prev := m.gameState
		if errors.Is(msg.Err, errNotRunning) {
			m.gameState = StateWaiting
			m.err = nil
			return m, nil
		}
		m.gameState = msg.State
		m.match = msg.Match
		m.pregame = msg.Pregame
		m.loadouts = m.match.summarize(m.primaryWeaponID)
		m.shard = msg.Shard
		m.err = msg.Err
		m.clampSkinsSelection()
		if subjects := m.matchSubjects(); m.cursor >= len(subjects) {
			m.cursor = max(len(subjects)-1, 0)
		}
		if prev == StateWaiting && m.err == nil {
			return m, tea.Batch(fetchMMRCmd, fetchPartyCmd, m.queuePlayerData(), m.requestSkinArt())
		}
		return m, tea.Batch(m.queuePlayerData(), m.requestSkinArt())

	case playerStatsMsg:
		m.playerStats[msg.Subject] = msg.Data
	case pollTickMsg:
		return m, tea.Batch(fetchGameStateCmd(), pollTickCmd())
	case namesMsg:
		// Merge, since party names and match names arrive from separate requests.
		if m.names == nil {
			m.names = map[string]string{}
		}
		for subject, name := range msg.Data {
			m.names[subject] = name
		}
		m.err = msg.Err
	case skinImageMsg:
		// A nil image marks the download as failed, so it isn't retried.
		m.skinImages[msg.URL] = msg.Img
		if msg.Err != nil {
			m.skinImages[msg.URL] = nil
		}
		return m, m.requestSkinArt()
	case weaponsMsg:
		m.weapons = msg.Data
		m.err = msg.Err
		for i, w := range m.weapons {
			if w.UUID == m.primaryWeaponID {
				m.settingsCursor = i
				break
			}
		}
	case tierNamesMsg:
		m.tierNames = msg.Data
		m.err = msg.Err

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

// statusBar shows the game state on the left and the shard on the right.
func (m model) statusBar() string {
	var left string
	switch m.gameState {
	case StateAgentSelect:
		left = agentSelectStatusStyle.Render("AGENT SELECT")
	case StateInGame:
		left = inGameStatusStyle.Render("IN-GAME")
	case StateWaiting:
		left = lobbyStatusStyle.Render("WAITING FOR GAME")
	default:
		left = lobbyStatusStyle.Render("LOBBY")
	}

	location := m.serverCity
	if location == "" {
		location = strings.ToUpper(m.shard)
	}
	if location == "" {
		location = "?"
	}
	right := shardChipStyle.Render(location)

	gapWidth := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gapWidth < 1 {
		gapWidth = 1
	}
	gap := statusBarBackground.Render(strings.Repeat(" ", gapWidth))

	return left + gap + right
}

// cityFromGamePod gets the city from a pod ID.
// "aresriot.aws-euc1-prod.eu-gp-frankfurt-1" becomes "Frankfurt".
func cityFromGamePod(pod string) string {
	parts := strings.Split(pod, ".")
	last := parts[len(parts)-1]

	segments := strings.Split(last, "-")
	if len(segments) < 2 {
		return pod
	}

	city := segments[len(segments)-2]
	if city == "" {
		return pod
	}

	return strings.ToUpper(city[:1]) + city[1:]
}

// tabBarHeight is the tab bar height in rows. Update uses it to detect tab clicks.
const tabBarHeight = 3

func (m model) View() string {
	hoveredTab := -1
	if m.hover.kind == hoverTab {
		hoveredTab = m.hover.index
	}
	tabBar, _ := buildTabBar(m.activeTab, hoveredTab, m.width, m.gameState != StateInGame)

	var content string
	switch m.activeTab {
	case tabMatch:
		content = m.playerSelectView()
	case tabSkins:
		content = m.skinsView()
	case tabSettings:
		content = m.settingsView()
	}

	// Pad between the content and the hint so the hint sits on the box's last line.
	content = strings.TrimRight(content, "\n")
	hint := m.hintText()
	gap := 1
	if m.height > 0 {
		gap = max(m.skinsContentHeight()-lipgloss.Height(content)-lipgloss.Height(hint), 1)
	}
	content += strings.Repeat("\n", gap+1) + hint

	box := boxStyle
	if m.width > 0 {
		// Border is 2 chars, padding is 4.
		box = box.Width(m.width - 6)
	}
	if m.height > 0 {
		// Subtract tab bar, its trailing newline, the status bar, and the box's own
		// top/bottom border so the whole layout fills the terminal instead of leaving
		// blank space below the status bar.
		box = box.Height(m.height - tabBarHeight - 1 - 1 - 2)
	}
	// A side effect, but only View knows where the skin image ends up.
	overlay.Set(m.skinOverlay())
	return tabBar + "\n" + box.Render(content) + "\n" + m.statusBar()
}

func (m model) playerSelectView() string {
	view, _ := m.buildMatchView()
	return view
}

// rosterSection is one titled table on the Match tab, like ALLIES or ENEMIES.
type rosterSection struct {
	title string
	rows  []rosterRow
}

// matchSections returns the tables for the current phase: your party in the lobby,
// your team in agent select, and allies plus enemies in game.
func (m model) matchSections() []rosterSection {
	switch m.gameState {
	case StateAgentSelect:
		return []rosterSection{{title: "YOUR TEAM", rows: m.pregame}}

	case StateInGame:
		myTeam := m.match.Teams[m.mySubject]
		var allies, enemies []rosterRow
		for _, l := range m.loadouts {
			skin := l.PrimarySkin
			if skin == "" {
				skin = "-"
			}
			level := m.match.Levels[l.Subject]
			if level == "" {
				level = "-"
			}
			row := rosterRow{Subject: l.Subject, Agent: l.AgentName, Level: level, Skin: skin, Incognito: m.match.Incognito[l.Subject]}
			if myTeam == "" || l.TeamID == myTeam {
				allies = append(allies, row)
			} else {
				enemies = append(enemies, row)
			}
		}
		if myTeam == "" {
			return []rosterSection{{title: "PLAYERS", rows: allies}}
		}
		return []rosterSection{{title: "ALLIES", rows: allies}, {title: "ENEMIES", rows: enemies}}

	default:
		return []rosterSection{{title: "PARTY", rows: partyRows(m.party)}}
	}
}

// matchSubjects lists every player on the Match tab in display order, for the cursor.
func (m model) matchSubjects() []string {
	var subjects []string
	for _, s := range m.matchSections() {
		for _, r := range s.rows {
			subjects = append(subjects, r.Subject)
		}
	}
	return subjects
}

// displayName is the name shown for a player. Players in streamer mode show their
// agent instead, unless they're you or in your party.
func (m model) displayName(r rosterRow, n int) string {
	if m.hidden(r) {
		if r.Agent != "" && r.Agent != "-" {
			return strings.TrimSuffix(r.Agent, "?")
		}
		return fmt.Sprintf("Player %d", n)
	}
	if name, ok := m.names[r.Subject]; ok {
		return name
	}
	return r.Subject
}

// queuePlayerData fetches names and stats for players on the Match tab that don't have
// them yet. Streamer mode players are skipped, so their data is never requested.
// It marks players as pending, so only call it from Update.
func (m model) queuePlayerData() tea.Cmd {
	var unknown []string
	var statsCmds []tea.Cmd
	for _, s := range m.matchSections() {
		for _, r := range s.rows {
			if m.hidden(r) {
				continue
			}
			if _, ok := m.names[r.Subject]; !ok {
				unknown = append(unknown, r.Subject)
			}
			// Each player's stats take ~7 requests, so fetch them once, one player at
			// a time, to stay under Riot's rate limit. Rows fill in as each arrives.
			if !m.statsPending[r.Subject] {
				m.statsPending[r.Subject] = true
				statsCmds = append(statsCmds, fetchPlayerStatsCmd(r.Subject))
			}
		}
	}

	var namesCmd tea.Cmd
	if len(unknown) > 0 {
		namesCmd = fetchNamesCmd(unknown)
	}
	return tea.Batch(namesCmd, tea.Sequence(statsCmds...))
}

// hidden reports whether a player's identity and stats must stay hidden. Riot's policy
// is that streamer mode players can't be identified by third-party tools, and rank or
// match history could identify them, so all of it is hidden. You and your party already
// know each other, so they're shown.
func (m model) hidden(r rosterRow) bool {
	return r.Incognito && r.Subject != m.mySubject && !m.inParty(r.Subject)
}

func (m model) inParty(subject string) bool {
	for _, r := range partyRows(m.party) {
		if r.Subject == subject {
			return true
		}
	}
	return false
}

// statCells formats a player's rank, peak rank, HS%, win rate and last RR change.
// "..." means still loading, "-" means no data.
func (m model) statCells(subject string) (rank, peak, hs, wr, delta string) {
	ps, ok := m.playerStats[subject]
	if !ok {
		return "...", "...", "...", "...", "..."
	}
	rank, peak, hs, wr, delta = "-", "-", "-", "-", "-"
	if ps.Err != nil {
		return
	}
	rank = m.tierLabel(ps.Tier)
	if ps.Tier > 0 {
		rank = fmt.Sprintf("%s (%.0f RR)", rank, ps.RR)
		delta = fmt.Sprintf("%+.0f", ps.DeltaRR)
	}
	peakTier := ps.PeakTier
	if subject == m.mySubject {
		// Your saved peak may be higher than what Riot's history still shows.
		peakTier = max(peakTier, m.peakTier)
	}
	peak = m.tierLabel(peakTier)
	if ps.Stats.Matches > 0 {
		hs = fmt.Sprintf("%.0f%%", ps.Stats.HSPercent)
		wr = fmt.Sprintf("%.0f%%", ps.Stats.WinRate)
	}
	return
}

// tierLabel names a competitive tier, like "DIAMOND 2". Tier 0 is unranked.
func (m model) tierLabel(tier int) string {
	if tier == 0 {
		return "Unranked"
	}
	if name, ok := m.tierNames[tier]; ok {
		return name
	}
	return fmt.Sprintf("Tier %d", tier)
}

// buildMatchView renders the Match tab and returns where each skin cell is drawn,
// so clicking one can open that player on the Skins tab.
func (m model) buildMatchView() (string, []skinsHitbox) {
	if m.gameState == StateWaiting {
		return m.spin.View() + " Waiting for game...\n", nil
	}

	if m.err != nil && m.party == nil && len(m.loadouts) == 0 {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err)) + "\n", nil
	}

	subjects := m.matchSubjects()
	selected := ""
	if m.cursor >= 0 && m.cursor < len(subjects) {
		selected = subjects[m.cursor]
	}

	var b strings.Builder
	var hits []skinsHitbox
	withSkin := m.gameState == StateInGame && !m.hiddenColumns["Skin"]
	n := 0
	for i, s := range m.matchSections() {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(weaponCategoryStyle.Render(s.title))
		b.WriteString("\n")
		if len(s.rows) == 0 {
			b.WriteString(m.spin.View())
			b.WriteString(" Loading...")
			continue
		}
		topY := settingsContentY + strings.Count(b.String(), "\n")
		table, tableHits := m.renderRosterTable(s.rows, selected, withSkin, topY, &n)
		b.WriteString(table)
		hits = append(hits, tableHits...)
	}

	return b.String() + "\n", hits
}

// hintText is the key help for the active tab. View pins it to the bottom of the box.
func (m model) hintText() string {
	switch m.activeTab {
	case tabSkins:
		return "(click or arrows: left/right switch column, up/down select)"
	case tabSettings:
		return "(tab: switch tabs, arrows: move, enter: select, or click)"
	}

	withSkin := m.gameState == StateInGame && !m.hiddenColumns["Skin"]
	hint := "(tab: switch tabs, up/down: select player)"
	if withSkin {
		hint = "(click a skin to see that player's loadout, or tab: switch tabs, up/down: select player)"
	}
	tip := weaponCategoryStyle.Render("Tip:") + " you can use the mouse. Click the tabs at the top to switch screens"
	if withSkin {
		tip += ", or click a skin to open that player's loadout"
	}
	return tip + ".\n" + hint
}

// renderRosterTable draws one player table. topY is the screen row of its top border.
// n numbers anonymous players across tables, so they read Player 1, Player 2, ...
func (m model) renderRosterTable(rows []rosterRow, selected string, withSkin bool, topY int, n *int) (string, []skinsHitbox) {
	withAgent := m.gameState != StateMenus
	var columns []string
	if withAgent {
		columns = append(columns, "Agent")
	}
	columns = append(columns, "Name")
	for _, c := range optionalColumns {
		if c != "Skin" && !m.hiddenColumns[c] {
			columns = append(columns, c)
		}
	}
	headers := append([]string{}, columns...)
	weaponColumn := "Vandal"
	for _, w := range m.weapons {
		if w.UUID == m.primaryWeaponID {
			weaponColumn = w.Name
			break
		}
	}
	if withSkin {
		headers = append(headers, weaponColumn)
	}
	skinCol := len(headers) - 1

	t := table.New().Headers(headers...)
	if m.width > 0 {
		t = t.Width(m.width - 10)
	}
	t = t.StyleFunc(func(row, col int) lipgloss.Style {
		if row == table.HeaderRow {
			return tableHeaderStyle
		}
		if row < 0 || row >= len(rows) {
			return normalRowStyle
		}
		style := normalRowStyle
		if rows[row].Subject == selected {
			style = selectedRowStyle
		}
		if withSkin && col == skinCol && m.hover == (hoverTarget{kind: hoverMatchSkin, subject: rows[row].Subject}) {
			style = style.Background(hoverBackground)
		}
		return style
	})

	for _, r := range rows {
		*n++
		rank, peak, hs, wr, delta := m.statCells(r.Subject)
		level := r.Level
		if m.hidden(r) {
			rank, peak, hs, wr, delta, level = "Hidden", "-", "-", "-", "-", "-"
		}
		values := map[string]string{
			"Agent": r.Agent, "Name": m.displayName(r, *n), "Rank": rank, "Peak Rank": peak,
			"HS": hs, "WR": wr, "Level": level, "ΔRR": delta,
		}
		var cells []string
		for _, c := range columns {
			cells = append(cells, values[c])
		}
		if withSkin {
			cells = append(cells, strings.TrimSuffix(r.Skin, " "+weaponColumn))
		}
		t.Row(cells...)
	}
	rendered := t.Render()
	if !withSkin {
		return rendered, nil
	}

	// The skin column sits between the header line's last column divider and its right
	// border. Lines are: top border, header, header divider, then one line per player.
	var hits []skinsHitbox
	lines := strings.Split(rendered, "\n")
	if len(lines) > 1 {
		header := []rune(ansi.Strip(lines[1]))
		right := len(header) - 1
		left := -1
		for i := right - 1; i >= 0; i-- {
			if header[i] == '│' {
				left = i
				break
			}
		}
		if left >= 0 {
			for i, r := range rows {
				hits = append(hits, skinsHitbox{
					subject: r.Subject,
					x0:      settingsContentX + left + 1,
					x1:      settingsContentX + right,
					y:       topY + 3 + i,
				})
			}
		}
	}
	return rendered, hits
}

// dumpDebugJSON writes raw API responses to disk, including a fresh match-history + one
// match-details fetch, for verifying unconfirmed field names. Temporary, for debugging.
func dumpDebugJSON(m model) error {
	dump := map[string]any{
		"party": m.party,
		"mmr":   m.mmr,
	}

	entitlements, shard, err := authenticate()
	if err == nil {
		if history, err := fetchMatchHistory(shard, entitlements.Subject, entitlements.AccessToken, entitlements.Token); err == nil {
			dump["matchHistory"] = history

			if entries, ok := history["History"].([]any); ok && len(entries) > 0 {
				if entry, ok := entries[0].(map[string]any); ok {
					if matchID, ok := entry["MatchID"].(string); ok {
						if match, err := fetchMatchDetails(shard, matchID, entitlements.AccessToken, entitlements.Token); err == nil {
							dump["matchDetails"] = match
						}
					}
				}
			}
		}
	}

	data, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("debug_dump.json", data, 0644)
}

func runTUI() {
	// Must run before Bubble Tea takes over stdin.
	graphics = detectGraphics()
	p := tea.NewProgram(newModel(), tea.WithAltScreen(), tea.WithMouseAllMotion(), tea.WithOutput(overlay))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

// errNotRunning means the lockfile is missing, i.e. the Riot Client isn't open yet.
var errNotRunning = errors.New("riot client is not running")

func authenticate() (EntitlementsToken, string, error) {
	path := os.Getenv("LOCALAPPDATA") + `\Riot Games\Riot Client\Config\lockfile`
	data, err := os.ReadFile(path)

	if err != nil {
		return EntitlementsToken{}, "", fmt.Errorf("%w: %w", errNotRunning, err)
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

	return mmrMsg{Data: mmr, Subject: entitlements.Subject}
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

// fetchNamesCmd wraps puuids in a closure since a tea.Cmd takes no arguments.
func fetchNamesCmd(puuids []string) tea.Cmd {
	return func() tea.Msg {
		entitlements, shard, err := authenticate()
		if err != nil {
			return namesMsg{Err: err}
		}

		names, err := fetchPlayerNames(shard, puuids, entitlements.AccessToken, entitlements.Token)
		if err != nil {
			return namesMsg{Err: err}
		}

		index := map[string]string{}
		for _, n := range names {
			index[n.Subject] = fmt.Sprintf("%s#%s", n.GameName, n.TagLine)
		}

		return namesMsg{Data: index}
	}
}

// fetchGameStateCmd tries core-game, then pregame. If both 404, the player is in menus.
func fetchGameStateCmd() tea.Cmd {
	return func() tea.Msg {
		entitlements, shard, err := authenticate()
		if err != nil {
			return gameStateMsg{Err: err}
		}

		if _, err := fetchPartyID(shard, entitlements.Subject, entitlements.AccessToken, entitlements.Token); err != nil {
			return gameStateMsg{State: StateWaiting, Shard: shard}
		}

		if matchID, err := fetchCoreGameMatchID(shard, entitlements.Subject, entitlements.AccessToken, entitlements.Token); err == nil {
			loadouts, err := fetchLoadouts(shard, matchID.MatchID, entitlements.AccessToken, entitlements.Token)
			if err != nil {
				return gameStateMsg{Err: err}
			}

			skins, err := fetchWeaponSkins()
			if err != nil {
				return gameStateMsg{Err: err}
			}
			skinIndex := buildSkinIndex(skins["data"].([]any))

			agents, err := fetchAgents()
			if err != nil {
				return gameStateMsg{Err: err}
			}
			agentIndex := buildAgentNameIndex(agents["data"].([]any))

			data := matchData{Loadouts: loadouts, SkinIndex: skinIndex, AgentIndex: agentIndex}

			if match, err := fetchCoreGameMatch(shard, matchID.MatchID, entitlements.AccessToken, entitlements.Token); err == nil {
				data.Teams = teamsFromMatch(match)
				data.Levels = levelsFromMatch(match)
				data.Incognito = incognitoFromMatch(match)
			}
			return gameStateMsg{State: StateInGame, Match: data, Shard: shard}
		}

		if matchID, err := fetchPregameMatchID(shard, entitlements.Subject, entitlements.AccessToken, entitlements.Token); err == nil {
			msg := gameStateMsg{State: StateAgentSelect, Shard: shard}
			// The roster is optional. Without it the table falls back to "Loading".
			match, err := fetchPregameMatch(shard, matchID.MatchID, entitlements.AccessToken, entitlements.Token)
			if err != nil {
				return msg
			}
			agentIndex := map[string]string{}
			if agents, err := fetchAgents(); err == nil {
				agentIndex = buildAgentNameIndex(agents["data"].([]any))
			}
			msg.Pregame = pregameRows(match, agentIndex)
			return msg
		}

		return gameStateMsg{State: StateMenus, Shard: shard}
	}
}
