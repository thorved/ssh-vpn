package userui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var (
	green         = lipgloss.AdaptiveColor{Light: "#087E5B", Dark: "#46D6A0"}
	cyan          = lipgloss.AdaptiveColor{Light: "#007C91", Dark: "#56D4DD"}
	blue          = lipgloss.AdaptiveColor{Light: "#2257A5", Dark: "#78A9FF"}
	amber         = lipgloss.AdaptiveColor{Light: "#9A5B00", Dark: "#F6C177"}
	purple        = lipgloss.AdaptiveColor{Light: "#6F42C1", Dark: "#C4A7E7"}
	pink          = lipgloss.AdaptiveColor{Light: "#B42370", Dark: "#F38BA8"}
	red           = lipgloss.AdaptiveColor{Light: "#B42318", Dark: "#FF7B72"}
	muted         = lipgloss.AdaptiveColor{Light: "#667085", Dark: "#89929B"}
	border        = lipgloss.AdaptiveColor{Light: "#CBD5E1", Dark: "#39424E"}
	selection     = lipgloss.AdaptiveColor{Light: "#E8F0FE", Dark: "#20304A"}
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(green)
	mutedStyle    = lipgloss.NewStyle().Foreground(muted)
	selectedStyle = lipgloss.NewStyle().Foreground(blue).Background(selection).Bold(true)
)

func init() {
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(true)
}

func (m model) View() string {
	width := m.width
	if width < 24 {
		width = 80
	}
	var body string
	if m.showHelp {
		body = m.help(width)
	} else {
		switch m.page {
		case overviewPage:
			body = m.overview(width)
		case publishingPage:
			body = m.publishing(width)
		case activityPage:
			body = m.activities(width)
		}
	}
	return m.header(width) + "\n" + body + "\n" + m.footer(width)
}

func (m model) header(width int) string {
	brand := titleStyle.Render("SSH VPN  ◆  ") + lipgloss.NewStyle().Bold(true).Foreground(cyan).Render(m.snapshot.Room)
	role := lipgloss.NewStyle().Bold(true).Foreground(roleColor(m.snapshot.Role)).Render(strings.ToUpper(m.snapshot.Role))
	updated := mutedStyle.Render("live · " + m.snapshot.CapturedAt.Local().Format("15:04:05"))
	line := brand + "  " + role
	if gap := width - lipgloss.Width(line) - lipgloss.Width(updated); gap > 1 {
		line += strings.Repeat(" ", gap) + updated
	}
	tabs := make([]string, len(pageNames))
	for i, name := range pageNames {
		name = displayPageName(name, width)
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if page(i) == m.page {
			tabs[i] = selectedStyle.Render(label)
		} else {
			tabs[i] = mutedStyle.Render(label)
		}
	}
	return line + "\n" + strings.Join(tabs, " ") + "\n" + m.toolbar() + "\n" + lipgloss.NewStyle().Foreground(border).Render(strings.Repeat("─", width))
}

func (m model) toolbar() string {
	items := make([]string, 0, len(m.toolbarItems()))
	for _, item := range m.toolbarItems() {
		style := lipgloss.NewStyle().Bold(true).Foreground(cyan)
		switch item.action {
		case "context", "confirm":
			style = lipgloss.NewStyle().Bold(true).Foreground(red)
		case "disconnect":
			style = lipgloss.NewStyle().Bold(true).Foreground(pink)
		case "cancel":
			style = mutedStyle
		case "help":
			style = lipgloss.NewStyle().Bold(true).Foreground(purple)
		}
		items = append(items, style.Render(item.label))
	}
	return strings.Join(items, "  ")
}

func (m model) overview(width int) string {
	items := []struct {
		label, value string
		color        lipgloss.TerminalColor
	}{
		{"ROLE", strings.ToUpper(m.snapshot.Role), roleColor(m.snapshot.Role)},
		{"PUBLISHED", fmt.Sprint(len(m.snapshot.Published)), amber},
		{"ACTIVE", fmt.Sprint(len(m.snapshot.Activities)), pink},
	}
	var cards string
	if width >= 90 {
		cardWidth := (width - 3) / 3
		cards = lipgloss.JoinHorizontal(lipgloss.Top, infoCard(items[0].label, items[0].value, items[0].color, cardWidth), " ", infoCard(items[1].label, items[1].value, items[1].color, cardWidth), " ", infoCard(items[2].label, items[2].value, items[2].color, cardWidth))
	} else {
		parts := make([]string, len(items))
		for i, item := range items {
			parts[i] = infoCard(item.label, item.value, item.color, width)
		}
		cards = strings.Join(parts, "\n")
	}
	ports := "none"
	if len(m.snapshot.Published) > 0 {
		values := make([]string, len(m.snapshot.Published))
		for i, item := range m.snapshot.Published {
			values[i] = fmt.Sprint(item.Port)
		}
		ports = strings.Join(values, ", ")
	}
	lines := []string{"", cards, "", titleStyle.Render("Your SSH session"),
		fmt.Sprintf("  Connection     #%d", m.snapshot.ConnectionID),
		fmt.Sprintf("  Room           %s", m.snapshot.Room),
		fmt.Sprintf("  Remote         %s", peer(m.snapshot.RemoteAddr)),
		fmt.Sprintf("  Connected      %s ago", age(m.snapshot.ConnectedAt)),
		fmt.Sprintf("  Publishing     %s", ports), "",
		mutedStyle.Render("Open the Publishing or Connections tab to manage your own resources.")}
	return strings.Join(lines, "\n")
}

func infoCard(label, value string, color lipgloss.TerminalColor, width int) string {
	content := lipgloss.NewStyle().Foreground(color).Bold(true).Render(value) + "  " + mutedStyle.Render(label)
	minimum := lipgloss.Width(content) + 4
	if width < minimum {
		width = minimum
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).Padding(0, 1).Width(width - 2).Height(1).Render(content)
}

func (m model) publishing(width int) string {
	lines := []string{titleStyle.Render(fmt.Sprintf("Your published ports (%d)", len(m.snapshot.Published))), mutedStyle.Render(truncate("    PORT     BIND TARGET                   ACTIVE CLIENTS     PUBLISHED", width))}
	if len(m.snapshot.Published) == 0 {
		return strings.Join(append(lines, "", mutedStyle.Render("  This SSH connection is not publishing a port.")), "\n")
	}
	start, end := visibleRange(m.cursor, len(m.snapshot.Published), m.bodyHeight()-3)
	for i := start; i < end; i++ {
		item := m.snapshot.Published[i]
		prefix, style := "  ", lipgloss.NewStyle().Foreground(amber)
		if i == m.cursor {
			prefix, style = "› ", selectedStyle
		}
		row := fmt.Sprintf(":%-7d %-29s %14d     %s", item.Port, truncate(defaultHost(item.BindHost), 29), item.ActiveClients, age(item.RegisteredAt))
		lines = append(lines, style.Render(prefix+truncate(row, width-2)))
	}
	return strings.Join(lines, "\n")
}

func (m model) activities(width int) string {
	lines := []string{titleStyle.Render(fmt.Sprintf("Your active connections (%d)", len(m.snapshot.Activities))), mutedStyle.Render(truncate("    ID      TYPE          PORT     PEER ADDRESS             CHANNELS   CONNECTED", width))}
	if len(m.snapshot.Activities) == 0 {
		return strings.Join(append(lines, "", mutedStyle.Render("  No traffic is currently flowing through this SSH connection.")), "\n")
	}
	start, end := visibleRange(m.cursor, len(m.snapshot.Activities), m.bodyHeight()-3)
	for i := start; i < end; i++ {
		item := m.snapshot.Activities[i]
		label, color := "TO PUBLISHER", cyan
		if item.Direction == "incoming" {
			label, color = "FROM CLIENT", purple
		}
		prefix, style := "  ", lipgloss.NewStyle().Foreground(color)
		if i == m.cursor {
			prefix, style = "› ", selectedStyle
		}
		row := fmt.Sprintf("#%-7d %-13s :%-7d %-24s %8d   %s", item.ID, label, item.Port, truncate(peer(item.PeerAddress), 24), item.ChannelCount, age(item.ConnectedAt))
		lines = append(lines, style.Render(prefix+truncate(row, width-2)))
	}
	return strings.Join(lines, "\n")
}

func (m model) help(width int) string {
	return lipgloss.NewStyle().Width(width).Render(strings.Join([]string{titleStyle.Render("Room dashboard help"), "", "  1–3 / Tab       switch view", "  ↑ ↓ / j k       select a port or connection", "  d / Enter       manage selected item", "  r               refresh live state", "  x / q           disconnect your SSH session", "  ?               toggle help", "", "  Mouse           click tabs, actions, rows, and use the wheel", "", mutedStyle.Render("Publishers can stop their ports and kick connected clients. Clients can close their own active traffic.")}, "\n"))
}

func (m model) footer(width int) string {
	line := mutedStyle.Render("Tab views  ·  ↑↓ select  ·  d manage  ·  r refresh  ·  x disconnect  ·  ? help")
	if m.confirm != nil {
		line = lipgloss.NewStyle().Foreground(red).Bold(true).Render("CONFIRM  ") + m.confirm.prompt + "  [y/Enter] confirm  [n/Esc] cancel"
	} else if m.status != "" {
		style := lipgloss.NewStyle().Foreground(green)
		if m.refreshing {
			style = lipgloss.NewStyle().Foreground(cyan).Bold(true)
		}
		if m.statusErr {
			style = lipgloss.NewStyle().Foreground(red)
		}
		line = style.Render(m.status)
	}
	return lipgloss.NewStyle().Foreground(border).Render(strings.Repeat("─", width)) + "\n" + lipgloss.NewStyle().MaxWidth(width).Render(line)
}

func roleColor(role string) lipgloss.TerminalColor {
	switch role {
	case "publisher+receiver":
		return purple
	case "publisher":
		return amber
	case "receiver":
		return cyan
	default:
		return muted
	}
}
func defaultHost(value string) string {
	if strings.TrimSpace(value) == "" {
		return "localhost"
	}
	return value
}
func age(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}
func visibleRange(cursor, length, capacity int) (int, int) {
	if capacity < 1 {
		capacity = 1
	}
	start := cursor - capacity + 1
	if start < 0 {
		start = 0
	}
	end := min(length, start+capacity)
	return start, end
}
func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
