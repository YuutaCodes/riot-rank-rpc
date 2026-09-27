package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	lipgloss "github.com/charmbracelet/lipgloss"
)

// settingsContentX and settingsContentY are where the box's content starts on screen:
// tab bar rows, then the box's top border and padding (1 row) / left border and padding (2 cols).
const (
	settingsContentX = 1 + 2
	settingsContentY = tabBarHeight + 1 + 1
)

// Includes a 2 column gap on the right, like the Skins tab panes.
const settingsCategoryPaneWidth = 22

const (
	settingsFocusCategories = iota
	settingsFocusContent
)

// Settings categories, in display order. The index matches model.settingsCategory.
const (
	settingsCategoryWeapon = iota
	settingsCategoryColumns
)

var settingsCategoryLabels = []string{"Primary weapon", "Columns"}

// optionalColumns are the Match tab columns that can be hidden. Agent and Name always show.
// The keys are saved in settings.json, so don't rename them.
var optionalColumns = []string{"Rank", "Peak Rank", "HS", "WR", "Level", "ΔRR", "Skin"}

// What a settingsHitbox points at.
const (
	settingsHitCategory = iota
	settingsHitWeapon
	settingsHitColumn
)

// settingsHitbox is a clickable item at row y, columns [x0, x1).
type settingsHitbox struct {
	kind   int
	index  int
	x0, x1 int
	y      int
}

// buildSettings renders the settings tab: categories on the left, the chosen category's
// options on the right. View and Update both use it, so mouse clicks always match what's drawn.
func (m model) buildSettings() (string, []settingsHitbox) {
	var hits []settingsHitbox

	categories := []string{weaponCategoryStyle.Render("SETTINGS")}
	for i, label := range settingsCategoryLabels {
		prefix, style := "  ", skinsRowStyle
		if i == m.settingsCategory {
			style = skinsSelectedStyle
			if m.settingsFocus == settingsFocusCategories {
				prefix = "> "
			}
		}
		if m.hover == (hoverTarget{kind: hoverSettingCategory, index: i}) {
			style = style.Background(hoverBackground)
		}
		categories = append(categories, prefix+style.Render(label))
		hits = append(hits, settingsHitbox{
			kind:  settingsHitCategory,
			index: i,
			x0:    settingsContentX,
			x1:    settingsContentX + settingsCategoryPaneWidth,
			y:     settingsContentY + 1 + i,
		})
	}
	left := lipgloss.NewStyle().Width(settingsCategoryPaneWidth).Render(strings.Join(categories, "\n"))

	var right string
	var rightHits []settingsHitbox
	switch m.settingsCategory {
	case settingsCategoryWeapon:
		right, rightHits = m.buildWeaponSettings()
	case settingsCategoryColumns:
		right, rightHits = m.buildColumnSettings()
	}
	hits = append(hits, rightHits...)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n", hits
}

// buildWeaponSettings renders the weapon grid, one column per category.
func (m model) buildWeaponSettings() (string, []settingsHitbox) {
	if len(m.weapons) == 0 {
		return m.spin.View() + " Loading weapons...\n", nil
	}

	var b strings.Builder
	b.WriteString(weaponCategoryStyle.Render("PRIMARY WEAPON"))
	b.WriteString("  shown as the skin column on the Match tab\n\n")
	paneX := settingsContentX + settingsCategoryPaneWidth
	gridY := settingsContentY + strings.Count(b.String(), "\n")

	var columns []string
	// colItems[c] holds (weapon index, line inside column c) for each weapon in it.
	var colItems [][][2]int
	var items [][2]int
	line := 0
	var col strings.Builder
	for i, w := range m.weapons {
		if i == 0 || w.Category != m.weapons[i-1].Category {
			if i > 0 {
				columns = append(columns, weaponColumnStyle.Render(col.String()))
				colItems = append(colItems, items)
				items, line = nil, 0
				col.Reset()
			}
			line++
			col.WriteString(weaponCategoryStyle.Render(strings.ToUpper(w.Category)))
			col.WriteString("\n")
		}

		prefix := "  "
		style := normalRowStyle
		if i == m.settingsCursor {
			style = selectedRowStyle
			if m.settingsFocus == settingsFocusContent {
				prefix = "> "
			}
		}
		if m.hover == (hoverTarget{kind: hoverSetting, index: i}) {
			style = style.Background(hoverBackground)
		}
		marker := ""
		if w.UUID == m.primaryWeaponID {
			marker = " *"
		}
		items = append(items, [2]int{i, line})
		line++
		col.WriteString(prefix)
		col.WriteString(style.Render(w.Name + marker))
		col.WriteString("\n")
	}
	columns = append(columns, weaponColumnStyle.Render(col.String()))
	colItems = append(colItems, items)

	maxWidth := m.width - 10 - settingsCategoryPaneWidth
	var rows []string
	var row []string
	var hits []settingsHitbox
	rowWidth := 0
	rowY := gridY
	for ci, c := range columns {
		w := lipgloss.Width(c)
		if len(row) > 0 && m.width > 0 && rowWidth+w > maxWidth {
			joined := lipgloss.JoinHorizontal(lipgloss.Top, row...)
			rows = append(rows, joined)
			rowY += lipgloss.Height(joined)
			row, rowWidth = nil, 0
		}
		x := paneX + rowWidth
		for _, it := range colItems[ci] {
			hits = append(hits, settingsHitbox{kind: settingsHitWeapon, index: it[0], x0: x, x1: x + w, y: rowY + it[1]})
		}
		row = append(row, c)
		rowWidth += w
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))
	b.WriteString(lipgloss.JoinVertical(lipgloss.Left, rows...))
	b.WriteString("\n* current")
	return b.String(), hits
}

// buildColumnSettings renders a checkbox per optional Match tab column.
func (m model) buildColumnSettings() (string, []settingsHitbox) {
	var b strings.Builder
	b.WriteString(weaponCategoryStyle.Render("COLUMNS"))
	b.WriteString("  shown on the Match tab\n\n")
	paneX := settingsContentX + settingsCategoryPaneWidth
	listY := settingsContentY + strings.Count(b.String(), "\n")

	var hits []settingsHitbox
	for i, c := range optionalColumns {
		prefix, style := "  ", normalRowStyle
		if i == m.columnCursor {
			style = selectedRowStyle
			if m.settingsFocus == settingsFocusContent {
				prefix = "> "
			}
		}
		if m.hover == (hoverTarget{kind: hoverSettingColumn, index: i}) {
			style = style.Background(hoverBackground)
		}
		box := "[x] "
		if m.hiddenColumns[c] {
			box = "[ ] "
		}
		label := c
		if c == "Skin" {
			label = "Skin (primary weapon, in game)"
		}
		b.WriteString(prefix)
		b.WriteString(style.Render(box + label))
		b.WriteString("\n")
		hits = append(hits, settingsHitbox{kind: settingsHitColumn, index: i, x0: paneX, x1: paneX + 40, y: listY + i})
	}
	return b.String(), hits
}

// handleSettingsKey handles keys on the settings tab. ok is false for keys it doesn't use.
func (m model) handleSettingsKey(key string) (_ tea.Model, _ tea.Cmd, ok bool) {
	if m.settingsFocus == settingsFocusCategories {
		switch key {
		case "up", "k":
			m.settingsCategory = max(m.settingsCategory-1, 0)
		case "down", "j":
			m.settingsCategory = min(m.settingsCategory+1, len(settingsCategoryLabels)-1)
		case "right", "l", "enter":
			m.settingsFocus = settingsFocusContent
		default:
			return m, nil, false
		}
		return m, nil, true
	}

	if m.settingsCategory == settingsCategoryColumns {
		switch key {
		case "up", "k":
			m.columnCursor = max(m.columnCursor-1, 0)
		case "down", "j":
			m.columnCursor = min(m.columnCursor+1, len(optionalColumns)-1)
		case "left", "h":
			m.settingsFocus = settingsFocusCategories
		case "enter", " ":
			return m.toggleColumn(m.columnCursor), nil, true
		default:
			return m, nil, false
		}
		return m, nil, true
	}

	switch key {
	case "up", "k":
		m.settingsCursor = max(m.settingsCursor-1, 0)
	case "down", "j":
		m.settingsCursor = min(m.settingsCursor+1, max(len(m.weapons)-1, 0))
	case "left", "h":
		// From the first weapon category, left goes back to the category list.
		if len(m.weapons) == 0 || m.weapons[m.settingsCursor].Category == m.weapons[0].Category {
			m.settingsFocus = settingsFocusCategories
		} else {
			m.settingsCursor = prevCategoryStart(m.weapons, m.settingsCursor)
		}
	case "right", "l":
		m.settingsCursor = nextCategoryStart(m.weapons, m.settingsCursor)
	case "enter":
		if m.settingsCursor < len(m.weapons) {
			next, cmd := m.setPrimaryWeapon(m.settingsCursor)
			return next, cmd, true
		}
	default:
		return m, nil, false
	}
	return m, nil, true
}

// handleSettingsClick selects or toggles whatever is at x, y on the settings tab.
func (m model) handleSettingsClick(x, y int) (tea.Model, tea.Cmd) {
	hits := m.hits.settings
	for _, h := range hits {
		if y != h.y || x < h.x0 || x >= h.x1 {
			continue
		}
		switch h.kind {
		case settingsHitCategory:
			m.settingsCategory = h.index
			m.settingsFocus = settingsFocusCategories
		case settingsHitWeapon:
			m.settingsCursor = h.index
			m.settingsFocus = settingsFocusContent
			return m.setPrimaryWeapon(h.index)
		case settingsHitColumn:
			m.columnCursor = h.index
			m.settingsFocus = settingsFocusContent
			return m.toggleColumn(h.index), nil
		}
		return m, nil
	}
	return m, nil
}

// settingsHoverAt maps a settings tab hitbox to a hover target.
func (m model) settingsHoverAt(x, y int) hoverTarget {
	for _, h := range m.hits.settings {
		if y != h.y || x < h.x0 || x >= h.x1 {
			continue
		}
		switch h.kind {
		case settingsHitCategory:
			return hoverTarget{kind: hoverSettingCategory, index: h.index}
		case settingsHitColumn:
			return hoverTarget{kind: hoverSettingColumn, index: h.index}
		default:
			return hoverTarget{kind: hoverSetting, index: h.index}
		}
	}
	return hoverTarget{}
}

// setPrimaryWeapon saves weapon i as the primary weapon and re-summarizes the cached match.
func (m model) setPrimaryWeapon(i int) (tea.Model, tea.Cmd) {
	m.primaryWeaponID = m.weapons[i].UUID
	m.err = m.saveSettings()
	m.loadouts = m.match.summarize(m.primaryWeaponID)
	return m, nil
}

// toggleColumn shows or hides optional column i and saves the choice.
func (m model) toggleColumn(i int) model {
	// Copy so the old model's map isn't changed underneath it.
	hidden := map[string]bool{}
	for k, v := range m.hiddenColumns {
		hidden[k] = v
	}
	c := optionalColumns[i]
	hidden[c] = !hidden[c]
	m.hiddenColumns = hidden
	m.err = m.saveSettings()
	return m
}

func (m model) saveSettings() error {
	var hidden []string
	for _, c := range optionalColumns {
		if m.hiddenColumns[c] {
			hidden = append(hidden, c)
		}
	}
	return saveSettings(Settings{PrimaryWeaponID: m.primaryWeaponID, HiddenColumns: hidden})
}

func nextCategoryStart(weapons []WeaponOption, i int) int {
	for j := i + 1; j < len(weapons); j++ {
		if weapons[j].Category != weapons[i].Category {
			return j
		}
	}
	return i
}

func prevCategoryStart(weapons []WeaponOption, i int) int {
	if i <= 0 || i >= len(weapons) {
		return 0
	}
	j := i
	for j > 0 && weapons[j-1].Category == weapons[i].Category {
		j--
	}
	if j == 0 {
		return 0
	}
	prev := weapons[j-1].Category
	for j > 0 && weapons[j-1].Category == prev {
		j--
	}
	return j
}
