package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	lipgloss "github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	skinsFocusPlayers = iota
	skinsFocusWeapons
)

// Column widths include a 2 column gap on the right.
const (
	skinsPlayerPaneWidth = 30
	skinsWeaponPaneWidth = 32
)

var (
	skinsRowStyle      = normalRowStyle.Padding(0)
	skinsSelectedStyle = selectedRowStyle.Padding(0)
)

// skinsRow is a line in the player column. Headers have no subject.
type skinsRow struct {
	label   string
	subject string
}

type weaponSkin struct {
	weapon string
	skin   SkinInfo
}

// skinsHitbox is a clickable row at y, columns [x0, x1). Weapon rows have no subject.
type skinsHitbox struct {
	subject string
	weapon  int
	x0, x1  int
	y       int
}

// skinsPlayerRows lists allies, then enemies, or one list if teams are unknown.
func (m model) skinsPlayerRows() []skinsRow {
	label := func(l PlayerLoadout) string {
		r := rosterRow{Subject: l.Subject, Agent: l.AgentName, Incognito: m.match.Incognito[l.Subject]}
		if m.hidden(r) {
			return fmt.Sprintf("%-9s%s", l.AgentName, "(hidden)")
		}
		return fmt.Sprintf("%-9s%s", l.AgentName, m.displayName(r, 0))
	}

	myTeam := ""
	for _, l := range m.loadouts {
		if l.Subject == m.mySubject {
			myTeam = l.TeamID
		}
	}

	if myTeam == "" {
		rows := []skinsRow{{label: "PLAYERS"}}
		for _, l := range m.loadouts {
			rows = append(rows, skinsRow{label: label(l), subject: l.Subject})
		}
		return rows
	}

	rows := []skinsRow{{label: "ALLIES"}}
	var enemies []skinsRow
	for _, l := range m.loadouts {
		row := skinsRow{label: label(l), subject: l.Subject}
		if l.TeamID == myTeam {
			rows = append(rows, row)
		} else {
			enemies = append(enemies, row)
		}
	}
	rows = append(rows, skinsRow{}, skinsRow{label: "ENEMIES"})
	return append(rows, enemies...)
}

func (m model) selectedLoadout() (PlayerLoadout, bool) {
	for _, l := range m.loadouts {
		if l.Subject == m.skinsSubject {
			return l, true
		}
	}
	return PlayerLoadout{}, false
}

func (m model) skinsWeaponRows(pl PlayerLoadout) []weaponSkin {
	var rows []weaponSkin
	for _, w := range m.weapons {
		if skin, ok := pl.WeaponSkins[w.UUID]; ok {
			rows = append(rows, weaponSkin{weapon: w.Name, skin: skin})
		}
	}
	return rows
}

// primaryWeaponRow returns the selected player's row for the primary weapon, or 0.
func (m model) primaryWeaponRow() int {
	pl, _ := m.selectedLoadout()
	i := 0
	for _, w := range m.weapons {
		if _, ok := pl.WeaponSkins[w.UUID]; !ok {
			continue
		}
		if w.UUID == m.primaryWeaponID {
			return i
		}
		i++
	}
	return 0
}

func (m model) selectedSkin() (SkinInfo, bool) {
	pl, ok := m.selectedLoadout()
	if !ok {
		return SkinInfo{}, false
	}
	rows := m.skinsWeaponRows(pl)
	if m.skinsWeapon < 0 || m.skinsWeapon >= len(rows) {
		return SkinInfo{}, false
	}
	return rows[m.skinsWeapon].skin, true
}

// clampSkinsSelection keeps the selection valid after the poll replaces m.loadouts.
func (m *model) clampSkinsSelection() {
	if _, ok := m.selectedLoadout(); !ok {
		m.skinsSubject = ""
		for _, l := range m.loadouts {
			if l.Subject == m.mySubject || m.skinsSubject == "" {
				m.skinsSubject = l.Subject
			}
		}
	}

	pl, _ := m.selectedLoadout()
	rows := m.skinsWeaponRows(pl)
	m.skinsWeapon = min(m.skinsWeapon, len(rows)-1)
	m.skinsWeapon = max(m.skinsWeapon, 0)
}

func (m *model) moveSkinsCursor(delta int) {
	if m.skinsFocus == skinsFocusWeapons {
		pl, _ := m.selectedLoadout()
		rows := m.skinsWeaponRows(pl)
		m.skinsWeapon = max(min(m.skinsWeapon+delta, len(rows)-1), 0)
		return
	}

	var subjects []string
	current := 0
	for _, r := range m.skinsPlayerRows() {
		if r.subject == "" {
			continue
		}
		if r.subject == m.skinsSubject {
			current = len(subjects)
		}
		subjects = append(subjects, r.subject)
	}
	if len(subjects) == 0 {
		return
	}
	next := max(min(current+delta, len(subjects)-1), 0)
	m.skinsSubject = subjects[next]
	// The weapon cursor stays put, so you can compare one gun across players.
	m.clampSkinsSelection()
}

// requestSkinArt renders the selected preview, or downloads the image first.
// It writes to the model's maps, so only call it from Update.
func (m model) requestSkinArt() tea.Cmd {
	skin, ok := m.selectedSkin()
	cols, rows := m.skinArtSize()
	if !ok || skin.Icon == "" || cols == 0 {
		return nil
	}

	key := skinArtKey(skin.Icon, cols, rows)
	if _, ok := m.skinArt[key]; ok {
		return nil
	}
	if img, ok := m.skinImages[skin.Icon]; ok {
		if img != nil && graphics.sixel {
			m.skinArt[key], _ = renderSixel(img, cols, rows)
		} else if img != nil {
			m.skinArt[key] = renderSkinArt(img, cols, rows)
		}
		return nil
	}
	if m.skinArtPending[skin.Icon] {
		return nil
	}
	m.skinArtPending[skin.Icon] = true
	return fetchSkinImageCmd(skin.Icon)
}

// skinArtSize returns 0, 0 when there's no room for a preview.
func (m model) skinArtSize() (int, int) {
	if m.width == 0 || m.height == 0 {
		return 40, 10
	}
	cols := min(m.width-10-skinsPlayerPaneWidth-skinsWeaponPaneWidth, skinArtMaxCols)
	rows := min(m.skinsContentHeight()-4, skinArtMaxRows)
	if cols < skinArtMinCols || rows < 2 {
		return 0, 0
	}
	return cols, rows
}

func skinArtKey(url string, cols, rows int) string {
	return fmt.Sprintf("%s@%dx%d", url, cols, rows)
}

// skinsContentHeight must match the box height in View.
func (m model) skinsContentHeight() int {
	return m.height - tabBarHeight - 1 - 1 - 2 - boxStyle.GetVerticalPadding()
}

func (m model) skinsWeaponWindow(total int) (int, int) {
	visible := total
	if m.height > 0 {
		// Leave room for the header, the blank line and the hint.
		visible = max(m.skinsContentHeight()-3, 1)
	}
	if total <= visible {
		return 0, total
	}
	start := min(max(m.skinsWeapon-visible/2, 0), total-visible)
	return start, start + visible
}

func (m model) skinsView() string {
	view, _ := m.buildSkins()
	return view
}

// buildSkins is shared by View and Update so clicks match what's drawn.
func (m model) buildSkins() (string, []skinsHitbox) {
	if m.gameState != StateInGame || len(m.loadouts) == 0 {
		return "Skins show up once a match has loaded.\n" +
			"Riot only shares other players' loadouts during a live match, not in the lobby or agent select.\n", nil
	}

	var hits []skinsHitbox

	var players []string
	for i, r := range m.skinsPlayerRows() {
		if r.subject == "" {
			players = append(players, weaponCategoryStyle.Render(r.label))
			continue
		}
		prefix, style := "  ", skinsRowStyle
		if r.subject == m.skinsSubject {
			style = skinsSelectedStyle
			if m.skinsFocus == skinsFocusPlayers {
				prefix = "> "
			}
		}
		if m.hover == (hoverTarget{kind: hoverSkinsPlayer, subject: r.subject}) {
			style = style.Background(hoverBackground)
		}
		players = append(players, prefix+style.Render(ansi.Truncate(r.label, skinsPlayerPaneWidth-4, "…")))
		hits = append(hits, skinsHitbox{
			subject: r.subject,
			x0:      settingsContentX,
			x1:      settingsContentX + skinsPlayerPaneWidth,
			y:       settingsContentY + i,
		})
	}

	pl, _ := m.selectedLoadout()
	rows := m.skinsWeaponRows(pl)
	start, end := m.skinsWeaponWindow(len(rows))
	header := weaponCategoryStyle.Render("LOADOUT")
	if end-start < len(rows) {
		header += skinsRowStyle.Faint(true).Render(fmt.Sprintf("  %d-%d of %d", start+1, end, len(rows)))
	}
	weapons := []string{header}
	if len(m.weapons) == 0 {
		weapons = append(weapons, m.spin.View()+" Loading weapons...")
	}
	for i := start; i < end; i++ {
		prefix, style := "  ", skinsRowStyle
		if i == m.skinsWeapon {
			style = skinsSelectedStyle
			if m.skinsFocus == skinsFocusWeapons {
				prefix = "> "
			}
		}
		if m.hover == (hoverTarget{kind: hoverSkinsWeapon, index: i}) {
			style = style.Background(hoverBackground)
		}
		rows[i].skin.Name = strings.TrimSuffix(rows[i].skin.Name, " "+rows[i].weapon)
		text := fmt.Sprintf("%-9s%s", rows[i].weapon, rows[i].skin.Name)
		weapons = append(weapons, prefix+style.Render(ansi.Truncate(text, skinsWeaponPaneWidth-4, "…")))
		hits = append(hits, skinsHitbox{
			weapon: i,
			x0:     settingsContentX + skinsPlayerPaneWidth,
			x1:     settingsContentX + skinsPlayerPaneWidth + skinsWeaponPaneWidth,
			y:      settingsContentY + 1 + i - start,
		})
	}

	panes := []string{
		lipgloss.NewStyle().Width(skinsPlayerPaneWidth).Render(strings.Join(players, "\n")),
		lipgloss.NewStyle().Width(skinsWeaponPaneWidth).Render(strings.Join(weapons, "\n")),
	}

	if cols, _ := m.skinArtSize(); cols > 0 {
		panes = append(panes, m.skinPreview())
	}

	hint := "(click or arrows: left/right switch column, up/down select)"
	return lipgloss.JoinHorizontal(lipgloss.Top, panes...) + "\n\n" + hint + "\n", hits
}

func (m model) skinPreview() string {
	skin, ok := m.selectedSkin()
	if !ok {
		return ""
	}

	cols, rows := m.skinArtSize()
	art, ok := m.skinArt[skinArtKey(skin.Icon, cols, rows)]
	img, downloaded := m.skinImages[skin.Icon]
	switch {
	case skin.Icon == "":
		art = "No image"
	case downloaded && img == nil:
		art = errorStyle.Render("Image unavailable")
	case !ok:
		art = m.spin.View() + " Loading image..."
	case graphics.sixel:
		// sixelOverlay draws the image over this blank area.
		art = strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", cols)+"\n", rows), "\n")
	}

	name := headerStyle.Render(ansi.Truncate(skin.Name, cols, "…"))
	return name + "\n\n" + art
}

// skinOverlay must match the preview position in buildSkins.
func (m model) skinOverlay() overlayImage {
	if !graphics.sixel || m.activeTab != tabSkins || m.gameState != StateInGame || len(m.loadouts) == 0 {
		return overlayImage{}
	}
	skin, ok := m.selectedSkin()
	cols, rows := m.skinArtSize()
	if !ok || cols == 0 {
		return overlayImage{}
	}
	seq := m.skinArt[skinArtKey(skin.Icon, cols, rows)]
	if seq == "" {
		return overlayImage{}
	}
	return overlayImage{
		seq:  seq,
		x:    settingsContentX + skinsPlayerPaneWidth + skinsWeaponPaneWidth,
		y:    settingsContentY + 2,
		cols: cols,
		rows: rows,
	}
}
