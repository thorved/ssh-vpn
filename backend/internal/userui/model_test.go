package userui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestRoleDashboardNavigationAndActions(t *testing.T) {
	store := &fakeStore{snapshot: sampleSnapshot()}
	m := newModel(store, make(chan struct{}), 110, 30)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m = updated.(model)
	if m.page != publishingPage {
		t.Fatalf("expected publishing page, got %d", m.page)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "8080") {
		t.Fatalf("missing stop confirmation: %#v", m.confirm)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "Kick client") {
		t.Fatalf("missing kick confirmation: %#v", m.confirm)
	}
}

func TestRoleDashboardRefreshMouseAndColor(t *testing.T) {
	store := &fakeStore{snapshot: sampleSnapshot()}
	m := newModel(store, make(chan struct{}), 110, 30)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updated.(model)
	if !m.refreshing || cmd == nil {
		t.Fatal("refresh did not start")
	}
	updated, _ = m.Update(snapshotMsg(store.snapshot))
	m = updated.(model)
	if m.refreshing || m.status != "" {
		t.Fatalf("refresh status stuck: %q", m.status)
	}

	connectionsX := len(" 1 Overview ") + 1 + len(" 2 Publishing ") + 2
	updated, _ = m.Update(tea.MouseMsg{X: connectionsX, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if m.page != activityPage {
		t.Fatalf("mouse did not select connections: %d", m.page)
	}
	if !strings.Contains(m.View(), "\x1b[38;5;") {
		t.Fatal("role dashboard did not emit ANSI-256 colors")
	}
}

func TestRoleOverviewNeverUsesLastTerminalColumn(t *testing.T) {
	m := newModel(&fakeStore{snapshot: sampleSnapshot()}, make(chan struct{}), 96, 30)
	for _, line := range strings.Split(m.overview(96), "\n") {
		if got := lipgloss.Width(line); got >= 96 {
			t.Fatalf("role overview line reaches or exceeds terminal edge: width=%d\n%s", got, line)
		}
	}
}

type fakeStore struct{ snapshot Snapshot }

func (f *fakeStore) Snapshot() Snapshot                   { return f.snapshot }
func (f *fakeStore) Subscribe() (<-chan struct{}, func()) { return make(chan struct{}), func() {} }
func (f *fakeStore) StopPublishing(port uint32) ActionResult {
	return ActionResult{Found: true, Message: fmt.Sprintf("stopped %d", port)}
}
func (f *fakeStore) CloseActivity(id uint64) ActionResult {
	return ActionResult{Found: true, Message: fmt.Sprintf("closed %d", id)}
}
func (f *fakeStore) Disconnect() ActionResult {
	return ActionResult{Found: true, Message: "bye", Disconnecting: true}
}

func sampleSnapshot() Snapshot {
	now := time.Now().Add(-time.Minute)
	return Snapshot{CapturedAt: time.Now(), Connected: true, Room: "ved", ConnectionID: 7, RemoteAddr: "10.0.0.1:5000", ConnectedAt: now, Role: "publisher", Published: []PublishedPort{{Port: 8080, BindHost: "localhost", RegisteredAt: now, ActiveClients: 1}}, Activities: []Activity{{ID: 9, Direction: "incoming", Port: 8080, PeerAddress: "10.0.0.2:6000", ConnectedAt: now, ChannelCount: 3}}}
}
