package adminui

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorved/ssh-vpn/backend/internal/admin"
)

type view int

const (
	overviewView view = iota
	roomsView
	connectionsView
	forwardsView
)

var viewNames = []string{"Overview", "Rooms", "Connections", "Forwards"}

type model struct {
	store      admin.Store
	events     <-chan struct{}
	snapshot   admin.Snapshot
	view       view
	cursor     int
	width      int
	height     int
	searching  bool
	query      string
	showHelp   bool
	confirm    *confirmation
	status     string
	statusErr  bool
	refreshing bool
}

type confirmation struct {
	prompt string
	run    func() admin.ActionResult
}

type snapshotMsg admin.Snapshot
type registryEventMsg struct{}
type refreshTickMsg time.Time
type actionMsg admin.ActionResult

func NewProgram(store admin.Store, input io.Reader, output io.Writer, width, height int) (*tea.Program, func()) {
	events, cancel := store.Subscribe()
	m := newModel(store, events, width, height)
	program := tea.NewProgram(m, tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen(), tea.WithMouseCellMotion())
	return program, cancel
}

func newModel(store admin.Store, events <-chan struct{}, width, height int) model {
	return model{store: store, events: events, width: width, height: height, snapshot: store.Snapshot()}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(waitForRegistryEvent(m.events), refreshTick())
}

func waitForRegistryEvent(events <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if events == nil {
			return nil
		}
		<-events
		return registryEventMsg{}
	}
}

func refreshTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return refreshTickMsg(t) })
}

func (m model) refresh() tea.Cmd {
	return func() tea.Msg { return snapshotMsg(m.store.Snapshot()) }
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case snapshotMsg:
		m.snapshot = admin.Snapshot(msg)
		if m.refreshing {
			m.refreshing = false
			m.status = ""
		}
		m.clampCursor()
		return m, nil
	case registryEventMsg:
		return m, tea.Batch(m.refresh(), waitForRegistryEvent(m.events))
	case refreshTickMsg:
		return m, tea.Batch(m.refresh(), refreshTick())
	case actionMsg:
		result := admin.ActionResult(msg)
		if result.Found {
			m.status = fmt.Sprintf("%s removed: %d connection(s), %d forward(s)", result.Target, result.ConnectionsClosed, result.PublishersRemoved)
			m.statusErr = false
		} else {
			m.status = result.Target + " was already gone"
			m.statusErr = true
		}
		m.confirm = nil
		return m, m.refresh()
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	}
	return m, nil
}

func (m model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm != nil {
		switch key.String() {
		case "y", "Y", "enter":
			action := m.confirm.run
			return m, func() tea.Msg { return actionMsg(action()) }
		case "n", "N", "esc", "q":
			m.confirm = nil
		}
		return m, nil
	}

	if m.searching {
		switch key.String() {
		case "enter":
			m.searching = false
			m.cursor = 0
		case "esc":
			m.searching = false
			m.query = ""
			m.cursor = 0
		case "backspace":
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
			}
		default:
			if key.Type == tea.KeyRunes {
				m.query += string(key.Runes)
			}
		}
		m.clampCursor()
		return m, nil
	}

	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
	case "tab", "right", "l":
		m.view = (m.view + 1) % 4
		m.cursor, m.query, m.showHelp = 0, "", false
	case "shift+tab", "left", "h":
		m.view = (m.view + 3) % 4
		m.cursor, m.query, m.showHelp = 0, "", false
	case "1", "2", "3", "4":
		index, _ := strconv.Atoi(key.String())
		m.view, m.cursor, m.query, m.showHelp = view(index-1), 0, "", false
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor+1 < m.itemCount() {
			m.cursor++
		}
	case "/":
		if m.view != overviewView {
			m.searching = true
		}
	case "r":
		m.status, m.statusErr, m.refreshing = "Refreshing live state…", false, true
		return m, m.refresh()
	case "d":
		m.prepareDelete()
	case "enter":
		m.showHelp = false
	}
	return m, nil
}

type toolbarItem struct {
	action string
	label  string
}

func (m model) toolbarItems() []toolbarItem {
	if m.confirm != nil {
		if m.width < 55 {
			return []toolbarItem{{"confirm", "[ Yes ]"}, {"cancel", "[ No ]"}}
		}
		return []toolbarItem{{"confirm", "[ Confirm removal ]"}, {"cancel", "[ Cancel ]"}}
	}
	if m.searching {
		if m.width < 55 {
			return []toolbarItem{{"apply", "[ Apply ]"}, {"clear", "[ Clear ]"}}
		}
		return []toolbarItem{{"apply", "[ Apply search ]"}, {"clear", "[ Clear ]"}}
	}
	if m.width < 55 {
		items := []toolbarItem{{"refresh", "[ R ]"}}
		if m.view != overviewView {
			items = append(items, toolbarItem{"remove", "[ D ]"})
		}
		return append(items, toolbarItem{"help", "[ ? ]"}, toolbarItem{"quit", "[ Q ]"})
	}
	items := []toolbarItem{{"refresh", "[ Refresh ]"}}
	if m.view != overviewView {
		items = append(items, toolbarItem{"remove", "[ Remove selected ]"})
	}
	return append(items, toolbarItem{"help", "[ Help ]"}, toolbarItem{"quit", "[ Quit ]"})
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if m.cursor+1 < m.itemCount() {
			m.cursor++
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if msg.Y == 1 {
		if selected, ok := m.tabAt(msg.X); ok {
			m.view, m.cursor, m.query, m.showHelp = selected, 0, "", false
		}
		return m, nil
	}
	if msg.Y == 2 {
		return m.handleToolbarClick(msg.X)
	}
	if m.confirm != nil || m.searching || m.showHelp {
		return m, nil
	}

	row := -1
	capacity := 0
	switch m.view {
	case roomsView:
		row = msg.Y - 5
		capacity = m.bodyHeight() - 2
		if m.width < 92 {
			capacity = max(4, m.bodyHeight()/2) - 2
		}
	case connectionsView, forwardsView:
		row = msg.Y - 6
		capacity = m.bodyHeight() - 3
	}
	if row < 0 || capacity <= 0 {
		return m, nil
	}
	start, end := visibleRange(m.cursor, m.itemCount(), capacity)
	index := start + row
	if index >= start && index < end {
		m.cursor = index
	}
	return m, nil
}

func (m model) handleToolbarClick(x int) (tea.Model, tea.Cmd) {
	action, ok := m.toolbarActionAt(x)
	if !ok {
		return m, nil
	}
	switch action {
	case "confirm":
		if m.confirm != nil {
			run := m.confirm.run
			return m, func() tea.Msg { return actionMsg(run()) }
		}
	case "cancel":
		m.confirm = nil
	case "apply":
		m.searching, m.cursor = false, 0
	case "clear":
		m.searching, m.query, m.cursor = false, "", 0
	case "refresh":
		m.status, m.statusErr, m.refreshing = "Refreshing live state…", false, true
		return m, m.refresh()
	case "remove":
		m.prepareDelete()
	case "help":
		m.showHelp = !m.showHelp
	case "quit":
		return m, tea.Quit
	}
	return m, nil
}

func (m model) toolbarActionAt(x int) (string, bool) {
	position := 0
	for _, item := range m.toolbarItems() {
		end := position + len(item.label)
		if x >= position && x < end {
			return item.action, true
		}
		position = end + 2
	}
	return "", false
}

func (m model) tabAt(x int) (view, bool) {
	position := 0
	for i, name := range viewNames {
		if m.width < 48 {
			name = string(name[0])
		}
		label := fmt.Sprintf(" %d %s ", i+1, name)
		end := position + len(label)
		if x >= position && x < end {
			return view(i), true
		}
		position = end + 1
	}
	return 0, false
}

func (m model) bodyHeight() int {
	height := m.height - 8
	if height < 8 {
		return 8
	}
	return height
}

func (m *model) prepareDelete() {
	switch m.view {
	case roomsView:
		rooms := m.filteredRooms()
		if len(rooms) == 0 {
			return
		}
		room := rooms[m.cursor]
		m.confirm = &confirmation{
			prompt: fmt.Sprintf("Remove room %q? This closes %d connection(s) and %d forward(s).", room.Name, room.ConnectionCount, room.PublisherCount),
			run:    func() admin.ActionResult { return m.store.DeleteRoom(room.Name) },
		}
	case connectionsView:
		connections := m.filteredConnections()
		if len(connections) == 0 {
			return
		}
		conn := connections[m.cursor]
		m.confirm = &confirmation{
			prompt: fmt.Sprintf("Disconnect #%d in %q? This also removes %d forward(s).", conn.ID, conn.Room, len(conn.PublishedPorts)),
			run:    func() admin.ActionResult { return m.store.DisconnectConnection(conn.ID) },
		}
	case forwardsView:
		forwards := m.filteredForwards()
		if len(forwards) == 0 {
			return
		}
		forward := forwards[m.cursor]
		owned := m.publisherCount(forward.ConnectionID)
		m.confirm = &confirmation{
			prompt: fmt.Sprintf("Remove %s:%d? Owner #%d will disconnect and all %d owned forward(s) will end.", forward.Room, forward.Port, forward.ConnectionID, owned),
			run:    func() admin.ActionResult { return m.store.DisconnectPublisher(forward.Room, forward.Port) },
		}
	}
}

func (m *model) clampCursor() {
	count := m.itemCount()
	if count == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= count {
		m.cursor = count - 1
	}
}

func (m model) itemCount() int {
	switch m.view {
	case roomsView:
		return len(m.filteredRooms())
	case connectionsView:
		return len(m.filteredConnections())
	case forwardsView:
		return len(m.filteredForwards())
	default:
		return 0
	}
}

func (m model) filteredRooms() []admin.Room {
	query := strings.ToLower(strings.TrimSpace(m.query))
	rooms := make([]admin.Room, 0, len(m.snapshot.Rooms))
	for _, room := range m.snapshot.Rooms {
		if query == "" || strings.Contains(strings.ToLower(room.Name), query) || roomMatches(room, query) {
			rooms = append(rooms, room)
		}
	}
	sort.SliceStable(rooms, func(i, j int) bool {
		if rooms[i].ActiveChannels != rooms[j].ActiveChannels {
			return rooms[i].ActiveChannels > rooms[j].ActiveChannels
		}
		if rooms[i].ConnectionCount != rooms[j].ConnectionCount {
			return rooms[i].ConnectionCount > rooms[j].ConnectionCount
		}
		return rooms[i].Name < rooms[j].Name
	})
	return rooms
}

func roomMatches(room admin.Room, query string) bool {
	for _, conn := range room.Connections {
		if strings.Contains(strings.ToLower(conn.RemoteAddr), query) {
			return true
		}
	}
	for _, pub := range room.Publishers {
		if strings.Contains(strconv.Itoa(int(pub.Port)), query) {
			return true
		}
	}
	return false
}

func (m model) filteredConnections() []admin.Connection {
	query := strings.ToLower(strings.TrimSpace(m.query))
	var out []admin.Connection
	for _, room := range m.snapshot.Rooms {
		for _, conn := range room.Connections {
			haystack := strings.ToLower(fmt.Sprintf("%d %s %s %s %v", conn.ID, conn.Room, conn.RemoteAddr, conn.Role, conn.PublishedPorts))
			if query == "" || strings.Contains(haystack, query) {
				out = append(out, conn)
			}
		}
	}
	return out
}

func (m model) filteredForwards() []admin.Publisher {
	query := strings.ToLower(strings.TrimSpace(m.query))
	var out []admin.Publisher
	for _, room := range m.snapshot.Rooms {
		for _, pub := range room.Publishers {
			haystack := strings.ToLower(fmt.Sprintf("%s %s %d %d %s", pub.Room, pub.BindHost, pub.Port, pub.ConnectionID, pub.RemoteAddr))
			if query == "" || strings.Contains(haystack, query) {
				out = append(out, pub)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Room != out[j].Room {
			return out[i].Room < out[j].Room
		}
		return out[i].Port < out[j].Port
	})
	return out
}

func (m model) publisherCount(connectionID uint64) int {
	count := 0
	for _, room := range m.snapshot.Rooms {
		for _, pub := range room.Publishers {
			if pub.ConnectionID == connectionID {
				count++
			}
		}
	}
	return count
}
