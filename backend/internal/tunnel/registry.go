package tunnel

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/thorved/ssh-vpn/backend/internal/admin"
	"golang.org/x/crypto/ssh"
)

var (
	ErrPublisherExists   = errors.New("publisher already exists")
	ErrPublisherNotFound = errors.New("publisher not found")
)

type Registry struct {
	mu          sync.RWMutex
	publishers  map[roomPort]*Publisher
	connections map[*ssh.ServerConn]*connectionRecord
	nextConnID  uint64
}

type Publisher struct {
	Room         string
	BindHost     string
	Port         uint32
	Conn         *ssh.ServerConn
	RegisteredAt time.Time
}

type roomPort struct {
	room string
	port uint32
}

type connectionRecord struct {
	ID                      uint64
	Room                    string
	RemoteAddr              string
	ConnectedAt             time.Time
	ActiveChannels          int
	ReceiverActiveChannels  int
	PublisherActiveChannels int
	ReceiverSeen            bool
	Conn                    *ssh.ServerConn
}

func NewRegistry() *Registry {
	return &Registry{
		publishers:  make(map[roomPort]*Publisher),
		connections: make(map[*ssh.ServerConn]*connectionRecord),
	}
}

func (r *Registry) RegisterConn(room string, conn *ssh.ServerConn, remoteAddr net.Addr) {
	if conn == nil {
		return
	}
	room = normalizeRoom(room)
	if room == "" {
		return
	}

	remote := ""
	if remoteAddr != nil {
		remote = remoteAddr.String()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextConnID++
	r.connections[conn] = &connectionRecord{
		ID:          r.nextConnID,
		Room:        room,
		RemoteAddr:  remote,
		ConnectedAt: time.Now().UTC(),
		Conn:        conn,
	}
}

func (r *Registry) Register(p *Publisher) error {
	if p == nil {
		return errors.New("publisher is nil")
	}
	room := normalizeRoom(p.Room)
	if room == "" {
		return errors.New("room is required")
	}
	if p.Port == 0 {
		return errors.New("port is required")
	}

	key := roomPort{room: room, port: p.Port}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.publishers[key]; exists {
		return fmt.Errorf("%w for room %q port %d", ErrPublisherExists, room, p.Port)
	}

	p.Room = room
	if p.RegisteredAt.IsZero() {
		p.RegisteredAt = time.Now().UTC()
	}
	r.publishers[key] = p
	return nil
}

func (r *Registry) Unregister(room string, port uint32, conn *ssh.ServerConn) bool {
	key := roomPort{room: normalizeRoom(room), port: port}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.publishers[key]
	if !exists {
		return false
	}
	if conn != nil && existing.Conn != conn {
		return false
	}

	delete(r.publishers, key)
	return true
}

func (r *Registry) Lookup(room string, port uint32) (*Publisher, error) {
	key := roomPort{room: normalizeRoom(room), port: port}

	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.publishers[key]
	if !exists {
		return nil, fmt.Errorf("%w for room %q port %d", ErrPublisherNotFound, key.room, port)
	}
	return p, nil
}

func (r *Registry) UnregisterConn(conn *ssh.ServerConn) int {
	if conn == nil {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	removed := 0
	for key, publisher := range r.publishers {
		if publisher.Conn == conn {
			delete(r.publishers, key)
			removed++
		}
	}
	delete(r.connections, conn)
	return removed
}

func (r *Registry) BeginForward(receiver *ssh.ServerConn, publisher *ssh.ServerConn) func() {
	r.mu.Lock()
	if record, exists := r.connections[receiver]; exists {
		record.ActiveChannels++
		record.ReceiverActiveChannels++
		record.ReceiverSeen = true
	}
	if publisher != nil && publisher != receiver {
		if record, exists := r.connections[publisher]; exists {
			record.ActiveChannels++
			record.PublisherActiveChannels++
		}
	}
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if record, exists := r.connections[receiver]; exists {
			if record.ActiveChannels > 0 {
				record.ActiveChannels--
			}
			if record.ReceiverActiveChannels > 0 {
				record.ReceiverActiveChannels--
			}
		}
		if publisher != nil && publisher != receiver {
			if record, exists := r.connections[publisher]; exists {
				if record.ActiveChannels > 0 {
					record.ActiveChannels--
				}
				if record.PublisherActiveChannels > 0 {
					record.PublisherActiveChannels--
				}
			}
		}
	}
}

func (r *Registry) DeleteRoom(room string) ([]*ssh.ServerConn, int) {
	room = normalizeRoom(room)
	if room == "" {
		return nil, 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	conns := make([]*ssh.ServerConn, 0)
	for conn, record := range r.connections {
		if record.Room == room {
			conns = append(conns, conn)
			delete(r.connections, conn)
		}
	}

	removedPublishers := 0
	for key := range r.publishers {
		if key.room == room {
			delete(r.publishers, key)
			removedPublishers++
		}
	}

	return conns, removedPublishers
}

func (r *Registry) DeleteConnection(id uint64, protectedRoom string) (*admin.ConnectionDeleteResult, bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for conn, record := range r.connections {
		if record.ID != id {
			continue
		}
		if record.Room == normalizeRoom(protectedRoom) {
			return nil, true, true
		}

		removedPublishers := 0
		for key, publisher := range r.publishers {
			if publisher.Conn == conn {
				delete(r.publishers, key)
				removedPublishers++
			}
		}
		delete(r.connections, conn)
		return &admin.ConnectionDeleteResult{
			Conn:              conn,
			ID:                record.ID,
			Room:              record.Room,
			RemovedPublishers: removedPublishers,
		}, true, false
	}

	return nil, false, false
}

func (r *Registry) Snapshot(publicDomain, publicSSHPort, adminUser string, adminDashboardPort uint32) admin.OverviewSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rooms := make(map[string]*admin.RoomSnapshot)
	publishedPorts := make(map[*ssh.ServerConn][]uint32)
	for _, publisher := range r.publishers {
		publishedPorts[publisher.Conn] = append(publishedPorts[publisher.Conn], publisher.Port)
	}
	for _, ports := range publishedPorts {
		sort.Slice(ports, func(i, j int) bool {
			return ports[i] < ports[j]
		})
	}

	for conn, record := range r.connections {
		room := snapshotRoom(rooms, record.Room)
		room.ConnectionCount++
		room.ActiveChannels += record.ActiveChannels
		ports := publishedPorts[conn]
		room.Connections = append(room.Connections, admin.ConnectionSnapshot{
			ID:                      record.ID,
			Room:                    record.Room,
			RemoteAddr:              record.RemoteAddr,
			ConnectedAt:             record.ConnectedAt.Format(time.RFC3339),
			ActiveChannels:          record.ActiveChannels,
			Role:                    connectionRole(record, len(ports) > 0),
			PublishedPorts:          append([]uint32(nil), ports...),
			ReceiverActiveChannels:  record.ReceiverActiveChannels,
			PublisherActiveChannels: record.PublisherActiveChannels,
		})
	}

	for _, publisher := range r.publishers {
		room := snapshotRoom(rooms, publisher.Room)
		room.PublisherCount++
		remoteAddr := ""
		var connectionID uint64
		if record, exists := r.connections[publisher.Conn]; exists {
			remoteAddr = record.RemoteAddr
			connectionID = record.ID
		}
		room.Publishers = append(room.Publishers, admin.PublisherSnapshot{
			BindHost:     publisher.BindHost,
			Port:         publisher.Port,
			ConnectionID: connectionID,
			RemoteAddr:   remoteAddr,
			RegisteredAt: publisher.RegisteredAt.Format(time.RFC3339),
		})
	}

	out := admin.OverviewSnapshot{
		PublicDomain:       publicDomain,
		PublicSSHPort:      publicSSHPort,
		AdminUser:          adminUser,
		AdminDashboardPort: adminDashboardPort,
		Rooms:              make([]admin.RoomSnapshot, 0, len(rooms)),
	}
	for _, room := range rooms {
		sort.Slice(room.Connections, func(i, j int) bool {
			return room.Connections[i].ID < room.Connections[j].ID
		})
		sort.Slice(room.Publishers, func(i, j int) bool {
			if room.Publishers[i].Port == room.Publishers[j].Port {
				return room.Publishers[i].BindHost < room.Publishers[j].BindHost
			}
			return room.Publishers[i].Port < room.Publishers[j].Port
		})
		out.Totals.Connections += room.ConnectionCount
		out.Totals.Publishers += room.PublisherCount
		out.Totals.ActiveChannels += room.ActiveChannels
		out.Rooms = append(out.Rooms, *room)
	}

	sort.Slice(out.Rooms, func(i, j int) bool {
		return out.Rooms[i].Name < out.Rooms[j].Name
	})
	out.Totals.Rooms = len(out.Rooms)

	return out
}

func snapshotRoom(rooms map[string]*admin.RoomSnapshot, name string) *admin.RoomSnapshot {
	room, exists := rooms[name]
	if !exists {
		room = &admin.RoomSnapshot{
			Name:        name,
			Connections: make([]admin.ConnectionSnapshot, 0),
			Publishers:  make([]admin.PublisherSnapshot, 0),
		}
		rooms[name] = room
	}
	return room
}

func normalizeRoom(room string) string {
	return strings.TrimSpace(room)
}

func connectionRole(record *connectionRecord, hasPublishedPorts bool) string {
	if hasPublishedPorts && record.ReceiverSeen {
		return "publisher+receiver"
	}
	if hasPublishedPorts {
		return "publisher"
	}
	if record.ReceiverSeen {
		return "receiver"
	}
	return "connected"
}
