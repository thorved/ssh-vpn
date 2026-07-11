package adminui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/thorved/ssh-vpn/backend/internal/admin"
)

func init() {
	// SSH channels are not os.File TTYs, so automatic detection otherwise
	// selects the no-color profile even when the client requested a color PTY.
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(true)
}

var (
	green         = lipgloss.AdaptiveColor{Light: "#087E5B", Dark: "#46D6A0"}
	blue          = lipgloss.AdaptiveColor{Light: "#2257A5", Dark: "#78A9FF"}
	cyan          = lipgloss.AdaptiveColor{Light: "#007C91", Dark: "#56D4DD"}
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
	dangerStyle   = lipgloss.NewStyle().Foreground(red).Bold(true)
)

func (m model) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	if width < 24 {
		width = 24
	}
	bodyHeight := m.bodyHeight()

	header := m.header(width)
	var body string
	if m.showHelp {
		body = m.help(width)
	} else {
		switch m.view {
		case overviewView:
			body = m.overview(width)
		case roomsView:
			body = m.rooms(width, bodyHeight)
		case connectionsView:
			body = m.connections(width, bodyHeight)
		case forwardsView:
			body = m.forwards(width, bodyHeight)
		}
	}
	footer := m.footer(width)
	return header + "\n" + body + "\n" + footer
}

func (m model) header(width int) string {
	brand := titleStyle.Render("SSH VPN  ◆  ADMIN")
	line := brand
	if width >= 52 {
		updated := mutedStyle.Render("live · " + m.snapshot.CapturedAt.Local().Format("15:04:05"))
		gap := width - lipgloss.Width(brand) - lipgloss.Width(updated)
		if gap < 1 {
			gap = 1
		}
		line += strings.Repeat(" ", gap) + updated
	}

	tabs := make([]string, 0, len(viewNames))
	for i, name := range viewNames {
		if width < 48 {
			name = string(name[0])
		}
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if view(i) == m.view {
			label = selectedStyle.Render(label)
		} else {
			label = mutedStyle.Render(label)
		}
		tabs = append(tabs, label)
	}
	return line + "\n" + strings.Join(tabs, " ") + "\n" + m.toolbar() + "\n" + lipgloss.NewStyle().Foreground(border).Render(strings.Repeat("─", width))
}

func (m model) toolbar() string {
	items := make([]string, 0, len(m.toolbarItems()))
	for _, item := range m.toolbarItems() {
		style := lipgloss.NewStyle().Bold(true).Foreground(cyan)
		switch item.action {
		case "remove", "confirm":
			style = lipgloss.NewStyle().Bold(true).Foreground(red)
		case "cancel", "clear", "quit":
			style = lipgloss.NewStyle().Foreground(muted)
		case "help":
			style = lipgloss.NewStyle().Bold(true).Foreground(purple)
		case "apply":
			style = lipgloss.NewStyle().Bold(true).Foreground(green)
		}
		items = append(items, style.Render(item.label))
	}
	return strings.Join(items, "  ")
}

func (m model) overview(width int) string {
	t := m.snapshot.Totals
	cards := []string{
		metric("ROOMS", t.Rooms, cyan), metric("CONNECTIONS", t.Connections, blue),
		metric("FORWARDS", t.Publishers, amber), metric("ACTIVE CHANNELS", t.ActiveChannels, pink),
	}
	var top string
	if width >= 100 {
		top = lipgloss.JoinHorizontal(lipgloss.Top, cards...)
	} else {
		top = strings.Join(cards, "\n")
	}

	lines := []string{"", top, "", titleStyle.Render("Most active rooms")}
	rooms := m.filteredRooms()
	if len(rooms) > 6 {
		rooms = rooms[:6]
	}
	if len(rooms) == 0 {
		lines = append(lines, mutedStyle.Render("  No tunnel rooms are connected."))
	}
	for _, room := range rooms {
		state := mutedStyle.Render("idle")
		if room.ActiveChannels > 0 {
			state = lipgloss.NewStyle().Foreground(green).Render("live")
		}
		lines = append(lines, fmt.Sprintf("  %-24s  %3d users  %3d forwards  %3d active  %s", truncate(room.Name, 24), room.ConnectionCount, room.PublisherCount, room.ActiveChannels, state))
	}
	return strings.Join(lines, "\n")
}

func metric(label string, value int, color lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 2).Width(19).
		Render(lipgloss.NewStyle().Foreground(color).Bold(true).Render(fmt.Sprintf("%-4d", value)) + " " + mutedStyle.Render(label))
}

func (m model) rooms(width, height int) string {
	rooms := m.filteredRooms()
	if len(rooms) == 0 {
		return emptyState("No rooms match the current filter.")
	}
	m.clampedSelection(len(rooms))
	room := rooms[m.cursor]
	if width < 92 {
		return m.roomList(rooms, width, max(4, height/2)) + "\n" + roomDetail(room, width)
	}
	leftWidth := min(42, width/3)
	left := m.roomList(rooms, leftWidth, height)
	right := roomDetail(room, width-leftWidth-3)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
}

func (m model) roomList(rooms []admin.Room, width, height int) string {
	lines := []string{titleStyle.Render(fmt.Sprintf("Rooms (%d)", len(rooms)))}
	start, end := visibleRange(m.cursor, len(rooms), height-2)
	for i := start; i < end; i++ {
		room := rooms[i]
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(roomColor(room))
		if i == m.cursor {
			prefix, style = "› ", selectedStyle
		}
		row := fmt.Sprintf("%-*s %2d/%2d/%2d", max(8, width-13), truncate(room.Name, max(8, width-13)), room.ConnectionCount, room.PublisherCount, room.ActiveChannels)
		lines = append(lines, style.Render(prefix+row))
	}
	return strings.Join(lines, "\n")
}

func roomDetail(room admin.Room, width int) string {
	lines := []string{
		titleStyle.Render(room.Name),
		fmt.Sprintf("%d connection(s)  ·  %d forward(s)  ·  %d active channel(s)", room.ConnectionCount, room.PublisherCount, room.ActiveChannels),
		"", mutedStyle.Render("Connections"),
	}
	if len(room.Connections) == 0 {
		lines = append(lines, "  none")
	}
	for _, conn := range room.Connections {
		lines = append(lines, fmt.Sprintf("  #%-4d %-20s %-18s %s", conn.ID, truncate(conn.RemoteAddr, 20), conn.Role, age(conn.ConnectedAt)))
	}
	lines = append(lines, "", mutedStyle.Render("Published forwards"))
	if len(room.Publishers) == 0 {
		lines = append(lines, "  none")
	}
	for _, pub := range room.Publishers {
		lines = append(lines, fmt.Sprintf("  :%-5d %-18s owner #%-4d %s", pub.Port, truncate(defaultHost(pub.BindHost), 18), pub.ConnectionID, age(pub.RegisteredAt)))
	}
	return lipgloss.NewStyle().Width(max(20, width)).Render(strings.Join(lines, "\n"))
}

func (m model) connections(width, height int) string {
	items := m.filteredConnections()
	lines := []string{titleStyle.Render(fmt.Sprintf("Connections (%d)", len(items))), mutedStyle.Render(truncate("    ID     ROOM                 REMOTE ADDRESS          ROLE                  UP       ACTIVE   PORTS", width))}
	if len(items) == 0 {
		return strings.Join(append(lines, "", emptyState("No connections match the current filter.")), "\n")
	}
	start, end := visibleRange(m.cursor, len(items), height-3)
	for i := start; i < end; i++ {
		conn := items[i]
		prefix, style := "  ", lipgloss.NewStyle().Foreground(roleColor(conn.Role))
		if i == m.cursor {
			prefix, style = "› ", selectedStyle
		}
		row := fmt.Sprintf("#%-5d %-20s %-23s %-21s %-8s %6d   %s", conn.ID, truncate(conn.Room, 20), truncate(conn.RemoteAddr, 23), conn.Role, age(conn.ConnectedAt), conn.ActiveChannels, ports(conn.PublishedPorts))
		lines = append(lines, style.Render(prefix+truncate(row, width-2)))
	}
	return strings.Join(lines, "\n")
}

func (m model) forwards(width, height int) string {
	items := m.filteredForwards()
	lines := []string{titleStyle.Render(fmt.Sprintf("Published forwards (%d)", len(items))), mutedStyle.Render(truncate("    ROOM                   PORT    BIND TARGET           OWNER    REMOTE ADDRESS         AGE       ACTIVE", width))}
	if len(items) == 0 {
		return strings.Join(append(lines, "", emptyState("No forwards match the current filter.")), "\n")
	}
	start, end := visibleRange(m.cursor, len(items), height-3)
	for i := start; i < end; i++ {
		pub := items[i]
		prefix, style := "  ", lipgloss.NewStyle().Foreground(cyan)
		if i == m.cursor {
			prefix, style = "› ", selectedStyle
		}
		row := fmt.Sprintf("%-22s :%-6d %-21s #%-7d %-22s %-9s %d", truncate(pub.Room, 22), pub.Port, truncate(defaultHost(pub.BindHost), 21), pub.ConnectionID, truncate(pub.RemoteAddr, 22), age(pub.RegisteredAt), pub.ActiveChannels)
		lines = append(lines, style.Render(prefix+truncate(row, width-2)))
	}
	return strings.Join(lines, "\n")
}

func (m model) help(width int) string {
	rows := []string{
		titleStyle.Render("Keyboard help"), "",
		"  1–4 / Tab       switch dashboard view", "  ↑ ↓ / j k       move selection",
		"  ← → / h l       switch dashboard view", "  /               search current view",
		"  d               remove selected item", "  r               refresh snapshot",
		"  ?               toggle this help", "  q / Ctrl+C      close dashboard",
		"", "  Mouse           click tabs, toolbar actions, and rows; wheel scrolls lists",
		"", mutedStyle.Render("All removals require confirmation. Admin sessions never appear in tunnel metrics."),
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(rows, "\n"))
}

func (m model) footer(width int) string {
	var line string
	if m.confirm != nil {
		line = dangerStyle.Render("CONFIRM  ") + m.confirm.prompt + "  " + dangerStyle.Render("[y/Enter] remove  [n/Esc] cancel")
	} else if m.searching {
		line = selectedStyle.Render("SEARCH  /") + m.query + "█  " + mutedStyle.Render("Enter apply · Esc clear")
	} else if m.status != "" {
		style := lipgloss.NewStyle().Foreground(green)
		if m.refreshing {
			style = lipgloss.NewStyle().Foreground(cyan).Bold(true)
		}
		if m.statusErr {
			style = lipgloss.NewStyle().Foreground(red)
		}
		line = style.Render(m.status)
	} else {
		line = mutedStyle.Render("Tab views  ·  ↑↓ select  ·  / search  ·  d remove  ·  r refresh  ·  ? help  ·  q quit")
	}
	return lipgloss.NewStyle().Foreground(border).Render(strings.Repeat("─", width)) + "\n" + lipgloss.NewStyle().MaxWidth(width).Render(line)
}

func (m model) clampedSelection(length int) int {
	if length == 0 {
		return 0
	}
	return min(m.cursor, length-1)
}
func emptyState(message string) string { return "\n  " + mutedStyle.Render(message) }
func roomColor(room admin.Room) lipgloss.TerminalColor {
	if room.ActiveChannels > 0 {
		return green
	}
	if room.PublisherCount > 0 {
		return amber
	}
	return muted
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
func defaultHost(host string) string {
	if strings.TrimSpace(host) == "" {
		return "localhost"
	}
	return host
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
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
func ports(values []uint32) string {
	if len(values) == 0 {
		return "—"
	}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprint(value)
	}
	return strings.Join(parts, ",")
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
