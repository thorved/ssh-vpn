package adminui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorved/ssh-vpn/backend/internal/admin"
)

func TestModelNavigationSearchAndConfirmation(t *testing.T) {
	store := &fakeStore{snapshot: sampleSnapshot()}
	events := make(chan struct{}, 1)
	m := newModel(store, events, 100, 30)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m = updated.(model)
	if m.view != roomsView {
		t.Fatalf("expected rooms view, got %d", m.view)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(model)
	for _, char := range "beta" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		m = updated.(model)
	}
	if got := m.filteredRooms(); len(got) != 1 || got[0].Name != "beta" {
		t.Fatalf("unexpected search result: %#v", got)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "beta") {
		t.Fatalf("missing confirmation: %#v", m.confirm)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(model)
	if m.confirm != nil {
		t.Fatal("confirmation was not cancelled")
	}
}

func TestModelActionAndResize(t *testing.T) {
	store := &fakeStore{snapshot: sampleSnapshot()}
	m := newModel(store, make(chan struct{}), 80, 24)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	m = updated.(model)
	if m.width != 132 || m.height != 40 {
		t.Fatalf("resize not applied: %dx%d", m.width, m.height)
	}
	updated, _ = m.Update(actionMsg(admin.ActionResult{Found: true, Target: "alpha", ConnectionsClosed: 1, PublishersRemoved: 2}))
	m = updated.(model)
	if !strings.Contains(m.status, "1 connection") || m.statusErr {
		t.Fatalf("unexpected status: %q", m.status)
	}
	if view := m.View(); !strings.Contains(view, "SSH VPN") || !strings.Contains(view, "ROOMS") {
		t.Fatalf("dashboard content missing: %q", view)
	}
	if !strings.Contains(m.View(), "\x1b[38;5;") {
		t.Fatal("dashboard did not render ANSI-256 foreground colors")
	}
}

func TestRefreshStatusClearsWhenSnapshotArrives(t *testing.T) {
	store := &fakeStore{snapshot: sampleSnapshot()}
	m := newModel(store, make(chan struct{}), 100, 30)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updated.(model)
	if !m.refreshing || m.status != "Refreshing live state…" || cmd == nil {
		t.Fatalf("refresh state was not started: %#v", m)
	}
	updated, _ = m.Update(snapshotMsg(store.snapshot))
	m = updated.(model)
	if m.refreshing || m.status != "" {
		t.Fatalf("refresh state did not clear: refreshing=%v status=%q", m.refreshing, m.status)
	}
}

func TestMouseTabsToolbarAndRows(t *testing.T) {
	store := &fakeStore{snapshot: sampleSnapshot()}
	m := newModel(store, make(chan struct{}), 100, 30)

	roomsX := len(" 1 Overview ") + 2
	updated, _ := m.Update(tea.MouseMsg{X: roomsX, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if m.view != roomsView {
		t.Fatalf("mouse did not select rooms tab: %d", m.view)
	}

	updated, _ = m.Update(tea.MouseMsg{X: 1, Y: 6, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if m.cursor != 1 {
		t.Fatalf("mouse did not select second room row: %d", m.cursor)
	}

	removeX := len("[ Refresh ]") + 3
	updated, _ = m.Update(tea.MouseMsg{X: removeX, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "beta") {
		t.Fatalf("mouse remove did not open confirmation: %#v", m.confirm)
	}
}

type fakeStore struct{ snapshot admin.Snapshot }

func (f *fakeStore) Snapshot() admin.Snapshot             { return f.snapshot }
func (f *fakeStore) Subscribe() (<-chan struct{}, func()) { return make(chan struct{}), func() {} }
func (f *fakeStore) DeleteRoom(room string) admin.ActionResult {
	return admin.ActionResult{Found: true, Target: room}
}
func (f *fakeStore) DisconnectConnection(id uint64) admin.ActionResult {
	return admin.ActionResult{Found: true, Target: fmt.Sprint(id)}
}
func (f *fakeStore) DisconnectPublisher(room string, port uint32) admin.ActionResult {
	return admin.ActionResult{Found: true, Target: fmt.Sprintf("%s:%d", room, port)}
}

func sampleSnapshot() admin.Snapshot {
	now := time.Now().Add(-time.Minute)
	return admin.Snapshot{CapturedAt: time.Now(), Totals: admin.Totals{Rooms: 2, Connections: 2, Publishers: 1}, Rooms: []admin.Room{
		{Name: "alpha", ConnectionCount: 1, PublisherCount: 1, Connections: []admin.Connection{{ID: 1, Room: "alpha", RemoteAddr: "10.0.0.1", ConnectedAt: now, Role: "publisher", PublishedPorts: []uint32{8080}}}, Publishers: []admin.Publisher{{Room: "alpha", Port: 8080, ConnectionID: 1, RegisteredAt: now}}},
		{Name: "beta", ConnectionCount: 1, Connections: []admin.Connection{{ID: 2, Room: "beta", RemoteAddr: "10.0.0.2", ConnectedAt: now, Role: "connected"}}, Publishers: []admin.Publisher{}},
	}}
}
