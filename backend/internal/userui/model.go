package userui

import (
	"fmt"
	"io"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type page int

const (
	overviewPage page = iota
	publishingPage
	activityPage
)

var pageNames = []string{"Overview", "Publishing", "Connections"}

type model struct {
	store      Store
	events     <-chan struct{}
	snapshot   Snapshot
	page       page
	cursor     int
	width      int
	height     int
	showHelp   bool
	confirm    *confirmation
	status     string
	statusErr  bool
	refreshing bool
}

type confirmation struct {
	prompt string
	run    func() ActionResult
}

type snapshotMsg Snapshot
type registryEventMsg struct{}
type refreshTickMsg time.Time
type actionMsg ActionResult

func NewProgram(store Store, input io.Reader, output io.Writer, width, height int) (*tea.Program, func()) {
	events, cancel := store.Subscribe()
	m := newModel(store, events, width, height)
	return tea.NewProgram(m, tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen(), tea.WithMouseCellMotion()), cancel
}

func newModel(store Store, events <-chan struct{}, width, height int) model {
	return model{store: store, events: events, width: width, height: height, snapshot: store.Snapshot()}
}

func (m model) Init() tea.Cmd { return tea.Batch(waitForEvent(m.events), refreshTick()) }

func waitForEvent(events <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if events == nil {
			return nil
		}
		if _, ok := <-events; !ok {
			return nil
		}
		return registryEventMsg{}
	}
}

func refreshTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return refreshTickMsg(t) })
}

func (m model) refresh() tea.Cmd { return func() tea.Msg { return snapshotMsg(m.store.Snapshot()) } }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case snapshotMsg:
		m.snapshot = Snapshot(msg)
		if m.refreshing {
			m.refreshing, m.status = false, ""
		}
		m.clampCursor()
		if !m.snapshot.Connected {
			return m, tea.Quit
		}
	case registryEventMsg:
		return m, tea.Batch(m.refresh(), waitForEvent(m.events))
	case refreshTickMsg:
		return m, tea.Batch(m.refresh(), refreshTick())
	case actionMsg:
		result := ActionResult(msg)
		m.confirm = nil
		m.status, m.statusErr = result.Message, !result.Found
		if result.Disconnecting {
			return m, tea.Quit
		}
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
			run := m.confirm.run
			return m, func() tea.Msg { return actionMsg(run()) }
		case "n", "N", "esc":
			m.confirm = nil
		}
		return m, nil
	}
	switch key.String() {
	case "q", "x", "ctrl+c":
		m.prepareDisconnect()
	case "?":
		m.showHelp = !m.showHelp
	case "tab", "right", "l":
		m.page, m.cursor, m.showHelp = (m.page+1)%3, 0, false
	case "shift+tab", "left", "h":
		m.page, m.cursor, m.showHelp = (m.page+2)%3, 0, false
	case "1", "2", "3":
		m.page, m.cursor, m.showHelp = page(int(key.Runes[0]-'1')), 0, false
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor+1 < m.itemCount() {
			m.cursor++
		}
	case "d", "enter":
		m.prepareContextAction()
	case "r":
		m.status, m.statusErr, m.refreshing = "Refreshing live state…", false, true
		return m, m.refresh()
	}
	return m, nil
}

func (m *model) prepareContextAction() {
	switch m.page {
	case publishingPage:
		if len(m.snapshot.Published) == 0 {
			return
		}
		item := m.snapshot.Published[m.cursor]
		m.confirm = &confirmation{prompt: fmt.Sprintf("Stop publishing port %d and close its %d active client(s)?", item.Port, item.ActiveClients), run: func() ActionResult { return m.store.StopPublishing(item.Port) }}
	case activityPage:
		if len(m.snapshot.Activities) == 0 {
			return
		}
		item := m.snapshot.Activities[m.cursor]
		if item.Direction == "incoming" {
			m.confirm = &confirmation{prompt: fmt.Sprintf("Kick client %s from port %d? Their SSH connection will close.", peer(item.PeerAddress), item.Port), run: func() ActionResult { return m.store.CloseActivity(item.ID) }}
		} else {
			m.confirm = &confirmation{prompt: fmt.Sprintf("Close your active traffic to port %d?", item.Port), run: func() ActionResult { return m.store.CloseActivity(item.ID) }}
		}
	}
}

func (m *model) prepareDisconnect() {
	m.confirm = &confirmation{prompt: "Disconnect your SSH session and close all of its forwards and traffic?", run: m.store.Disconnect}
}

type toolbarItem struct{ action, label string }

func (m model) toolbarItems() []toolbarItem {
	if m.confirm != nil {
		if m.width < 70 {
			return []toolbarItem{{"confirm", "[ Yes ]"}, {"cancel", "[ No ]"}}
		}
		return []toolbarItem{{"confirm", "[ Confirm ]"}, {"cancel", "[ Cancel ]"}}
	}
	if m.width < 70 {
		items := []toolbarItem{{"refresh", "[ R ]"}}
		if m.page == publishingPage && len(m.snapshot.Published) > 0 {
			items = append(items, toolbarItem{"context", "[ Stop ]"})
		}
		if m.page == activityPage && len(m.snapshot.Activities) > 0 {
			label := "[ Close ]"
			if m.snapshot.Activities[m.cursor].Direction == "incoming" {
				label = "[ Kick ]"
			}
			items = append(items, toolbarItem{"context", label})
		}
		return append(items, toolbarItem{"disconnect", "[ Exit ]"}, toolbarItem{"help", "[ ? ]"})
	}
	items := []toolbarItem{{"refresh", "[ Refresh ]"}}
	if m.page == publishingPage && len(m.snapshot.Published) > 0 {
		items = append(items, toolbarItem{"context", "[ Stop port ]"})
	}
	if m.page == activityPage && len(m.snapshot.Activities) > 0 {
		label := "[ Close traffic ]"
		if m.snapshot.Activities[m.cursor].Direction == "incoming" {
			label = "[ Kick client ]"
		}
		items = append(items, toolbarItem{"context", label})
	}
	return append(items, toolbarItem{"disconnect", "[ Disconnect me ]"}, toolbarItem{"help", "[ Help ]"})
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
		if selected, ok := m.pageAt(msg.X); ok {
			m.page, m.cursor, m.showHelp = selected, 0, false
		}
		return m, nil
	}
	if msg.Y == 2 {
		return m.handleToolbar(msg.X)
	}
	if m.confirm != nil || m.showHelp || m.page == overviewPage {
		return m, nil
	}
	row := msg.Y - 6
	if row < 0 {
		return m, nil
	}
	start, end := visibleRange(m.cursor, m.itemCount(), m.bodyHeight()-3)
	index := start + row
	if index >= start && index < end {
		m.cursor = index
	}
	return m, nil
}

func (m model) handleToolbar(x int) (tea.Model, tea.Cmd) {
	action, ok := m.toolbarActionAt(x)
	if !ok {
		return m, nil
	}
	switch action {
	case "confirm":
		run := m.confirm.run
		return m, func() tea.Msg { return actionMsg(run()) }
	case "cancel":
		m.confirm = nil
	case "refresh":
		m.status, m.statusErr, m.refreshing = "Refreshing live state…", false, true
		return m, m.refresh()
	case "context":
		m.prepareContextAction()
	case "disconnect":
		m.prepareDisconnect()
	case "help":
		m.showHelp = !m.showHelp
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

func (m model) pageAt(x int) (page, bool) {
	position := 0
	for i, name := range pageNames {
		name = displayPageName(name, m.width)
		label := fmt.Sprintf(" %d %s ", i+1, name)
		end := position + len(label)
		if x >= position && x < end {
			return page(i), true
		}
		position = end + 1
	}
	return 0, false
}

func displayPageName(name string, width int) string {
	if width < 48 {
		return string(name[0])
	}
	return name
}

func (m *model) clampCursor() {
	count := m.itemCount()
	if count == 0 {
		m.cursor = 0
	} else if m.cursor >= count {
		m.cursor = count - 1
	}
}
func (m model) itemCount() int {
	if m.page == publishingPage {
		return len(m.snapshot.Published)
	}
	if m.page == activityPage {
		return len(m.snapshot.Activities)
	}
	return 0
}
func (m model) bodyHeight() int {
	height := m.height - 8
	if height < 8 {
		return 8
	}
	return height
}
func peer(value string) string {
	if value == "" {
		return "unknown peer"
	}
	return value
}
