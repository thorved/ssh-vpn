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
	"github.com/thorved/ssh-vpn/backend/internal/userui"
	"golang.org/x/crypto/ssh"
)

var (
	ErrPublisherExists   = errors.New("publisher already exists")
	ErrPublisherNotFound = errors.New("publisher not found")
)

type Registry struct {
	mu            sync.RWMutex
	publishers    map[roomPort]*Publisher
	connections   map[*ssh.ServerConn]*connectionRecord
	watchers      map[uint64]chan struct{}
	forwards      map[uint64]*activeForward
	nextConnID    uint64
	nextWatchID   uint64
	nextForwardID uint64
}

type Publisher struct {
	Room           string
	BindHost       string
	Port           uint32
	Conn           *ssh.ServerConn
	RegisteredAt   time.Time
	ActiveChannels int
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

type activeForward struct {
	ID            uint64
	Receiver      *ssh.ServerConn
	Publisher     *Publisher
	LocalChannel  ssh.Channel
	RemoteChannel ssh.Channel
	ConnectedAt   time.Time
}

func NewRegistry() *Registry {
	return &Registry{
		publishers:  make(map[roomPort]*Publisher),
		connections: make(map[*ssh.ServerConn]*connectionRecord),
		watchers:    make(map[uint64]chan struct{}),
		forwards:    make(map[uint64]*activeForward),
	}
}

func (r *Registry) Subscribe() (<-chan struct{}, func()) {
	r.mu.Lock()
	r.nextWatchID++
	id := r.nextWatchID
	ch := make(chan struct{}, 1)
	r.watchers[id] = ch
	r.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.watchers, id)
			close(ch)
			r.mu.Unlock()
		})
	}
}

func (r *Registry) notifyLocked() {
	for _, watcher := range r.watchers {
		select {
		case watcher <- struct{}{}:
		default:
		}
	}
}

func (r *Registry) RegisterConn(room string, conn *ssh.ServerConn, remoteAddr net.Addr) uint64 {
	if conn == nil || normalizeRoom(room) == "" {
		return 0
	}
	remote := ""
	if remoteAddr != nil {
		remote = remoteAddr.String()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextConnID++
	r.connections[conn] = &connectionRecord{
		ID: r.nextConnID, Room: normalizeRoom(room), RemoteAddr: remote,
		ConnectedAt: time.Now().UTC(), Conn: conn,
	}
	r.notifyLocked()
	return r.nextConnID
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
	r.notifyLocked()
	return nil
}

func (r *Registry) Unregister(room string, port uint32, conn *ssh.ServerConn) bool {
	key := roomPort{room: normalizeRoom(room), port: port}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, exists := r.publishers[key]
	if !exists || (conn != nil && existing.Conn != conn) {
		return false
	}
	delete(r.publishers, key)
	r.notifyLocked()
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
	_, existed := r.connections[conn]
	removed := r.removeConnectionLocked(conn)
	if removed > 0 || existed {
		r.notifyLocked()
	}
	return removed
}

func (r *Registry) removeConnectionLocked(conn *ssh.ServerConn) int {
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

func (r *Registry) BeginForward(receiver *ssh.ServerConn, publisher *Publisher, channels ...ssh.Channel) func() {
	r.mu.Lock()
	r.nextForwardID++
	forwardID := r.nextForwardID
	forward := &activeForward{ID: forwardID, Receiver: receiver, Publisher: publisher, ConnectedAt: time.Now().UTC()}
	if len(channels) > 0 {
		forward.LocalChannel = channels[0]
	}
	if len(channels) > 1 {
		forward.RemoteChannel = channels[1]
	}
	r.forwards[forwardID] = forward
	if record := r.connections[receiver]; record != nil {
		record.ActiveChannels++
		record.ReceiverActiveChannels++
		record.ReceiverSeen = true
	}
	if publisher != nil {
		publisher.ActiveChannels++
	}
	if publisher != nil && publisher.Conn != receiver {
		if record := r.connections[publisher.Conn]; record != nil {
			record.ActiveChannels++
			record.PublisherActiveChannels++
		}
	}
	r.notifyLocked()
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.forwards, forwardID)
			decrementForward(r.connections[receiver], true)
			if publisher != nil && publisher.ActiveChannels > 0 {
				publisher.ActiveChannels--
			}
			if publisher != nil && publisher.Conn != receiver {
				decrementForward(r.connections[publisher.Conn], false)
			}
			r.notifyLocked()
			r.mu.Unlock()
		})
	}
}

func (r *Registry) UserSnapshot(conn *ssh.ServerConn) userui.Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := userui.Snapshot{CapturedAt: time.Now().UTC()}
	record := r.connections[conn]
	if record == nil {
		return out
	}
	out.Connected = true
	out.Room, out.ConnectionID, out.RemoteAddr, out.ConnectedAt = record.Room, record.ID, record.RemoteAddr, record.ConnectedAt
	activeClients := make(map[*Publisher]map[*ssh.ServerConn]struct{})
	for _, forward := range r.forwards {
		if forward.Publisher == nil {
			continue
		}
		clients := activeClients[forward.Publisher]
		if clients == nil {
			clients = make(map[*ssh.ServerConn]struct{})
			activeClients[forward.Publisher] = clients
		}
		clients[forward.Receiver] = struct{}{}
	}
	for _, publisher := range r.publishers {
		if publisher.Conn != conn {
			continue
		}
		out.Published = append(out.Published, userui.PublishedPort{Port: publisher.Port, BindHost: publisher.BindHost, RegisteredAt: publisher.RegisteredAt, ActiveClients: len(activeClients[publisher])})
	}
	activityIndex := make(map[string]int)
	for _, forward := range r.forwards {
		if forward.Publisher == nil {
			continue
		}
		activity := userui.Activity{ID: forward.ID, Port: forward.Publisher.Port, ConnectedAt: forward.ConnectedAt, ChannelCount: 1}
		var peerID uint64
		switch {
		case forward.Publisher.Conn == conn:
			activity.Direction = "incoming"
			if peer := r.connections[forward.Receiver]; peer != nil {
				activity.PeerAddress = peer.RemoteAddr
				peerID = peer.ID
			}
		case forward.Receiver == conn:
			activity.Direction = "outgoing"
			if peer := r.connections[forward.Publisher.Conn]; peer != nil {
				activity.PeerAddress = peer.RemoteAddr
				peerID = peer.ID
			}
		default:
			continue
		}
		key := fmt.Sprintf("%s:%d:%d", activity.Direction, activity.Port, peerID)
		if index, exists := activityIndex[key]; exists {
			out.Activities[index].ChannelCount++
			if activity.ConnectedAt.Before(out.Activities[index].ConnectedAt) {
				out.Activities[index].ConnectedAt = activity.ConnectedAt
			}
			continue
		}
		activityIndex[key] = len(out.Activities)
		out.Activities = append(out.Activities, activity)
	}
	sort.Slice(out.Published, func(i, j int) bool { return out.Published[i].Port < out.Published[j].Port })
	sort.Slice(out.Activities, func(i, j int) bool { return out.Activities[i].ID < out.Activities[j].ID })
	out.Role = connectionRole(record, len(out.Published) > 0)
	return out
}

func (r *Registry) StopPublishing(conn *ssh.ServerConn, port uint32) userui.ActionResult {
	r.mu.Lock()
	record := r.connections[conn]
	if record == nil {
		r.mu.Unlock()
		return userui.ActionResult{Message: "SSH connection is no longer active"}
	}
	key := roomPort{room: record.Room, port: port}
	publisher := r.publishers[key]
	if publisher == nil || publisher.Conn != conn {
		r.mu.Unlock()
		return userui.ActionResult{Message: fmt.Sprintf("port %d is not published by this connection", port)}
	}
	delete(r.publishers, key)
	channels := r.forwardChannelsLocked(func(forward *activeForward) bool { return forward.Publisher == publisher })
	r.notifyLocked()
	r.mu.Unlock()
	closeForwardChannels(channels)
	return userui.ActionResult{Found: true, Message: fmt.Sprintf("stopped publishing port %d", port)}
}

func (r *Registry) CloseUserActivity(conn *ssh.ServerConn, id uint64) userui.ActionResult {
	r.mu.RLock()
	forward := r.forwards[id]
	if forward == nil {
		r.mu.RUnlock()
		return userui.ActionResult{Message: fmt.Sprintf("activity %d is no longer active", id)}
	}
	if forward.Publisher != nil && forward.Publisher.Conn == conn {
		receiver := forward.Receiver
		r.mu.RUnlock()
		if receiver != nil {
			_ = receiver.Close()
		}
		return userui.ActionResult{Found: true, Message: fmt.Sprintf("kicked client on port %d", forward.Publisher.Port)}
	}
	if forward.Receiver != conn {
		r.mu.RUnlock()
		return userui.ActionResult{Message: "that activity does not belong to this connection"}
	}
	channels := r.forwardChannelsLocked(func(candidate *activeForward) bool {
		return candidate.Receiver == conn && candidate.Publisher == forward.Publisher
	})
	port := forward.Publisher.Port
	r.mu.RUnlock()
	closeForwardChannels(channels)
	return userui.ActionResult{Found: true, Message: fmt.Sprintf("closed %d traffic channel(s) to port %d", len(channels)/2, port)}
}

func (r *Registry) forwardChannelsLocked(match func(*activeForward) bool) []ssh.Channel {
	var channels []ssh.Channel
	for _, forward := range r.forwards {
		if match(forward) {
			channels = append(channels, forward.LocalChannel, forward.RemoteChannel)
		}
	}
	return channels
}

func closeForwardChannels(channels []ssh.Channel) {
	for _, channel := range channels {
		if channel != nil {
			_ = channel.Close()
		}
	}
}

func decrementForward(record *connectionRecord, receiver bool) {
	if record == nil {
		return
	}
	if record.ActiveChannels > 0 {
		record.ActiveChannels--
	}
	if receiver && record.ReceiverActiveChannels > 0 {
		record.ReceiverActiveChannels--
	}
	if !receiver && record.PublisherActiveChannels > 0 {
		record.PublisherActiveChannels--
	}
}

func (r *Registry) DeleteRoom(room string) admin.ActionResult {
	room = normalizeRoom(room)
	result := admin.ActionResult{Target: room}
	if room == "" {
		return result
	}

	r.mu.Lock()
	conns := make([]*ssh.ServerConn, 0)
	for conn, record := range r.connections {
		if record.Room == room {
			conns = append(conns, conn)
			result.PublishersRemoved += r.removeConnectionLocked(conn)
		}
	}
	for key := range r.publishers {
		if key.room == room {
			delete(r.publishers, key)
			result.PublishersRemoved++
		}
	}
	result.ConnectionsClosed = len(conns)
	result.Found = result.ConnectionsClosed > 0 || result.PublishersRemoved > 0
	if result.Found {
		r.notifyLocked()
	}
	r.mu.Unlock()
	closeConnections(conns)
	return result
}

func (r *Registry) DisconnectConnection(id uint64) admin.ActionResult {
	result := admin.ActionResult{Target: fmt.Sprintf("connection %d", id)}
	r.mu.Lock()
	var target *ssh.ServerConn
	for conn, record := range r.connections {
		if record.ID == id {
			target = conn
			result.Found = true
			result.ConnectionsClosed = 1
			result.PublishersRemoved = r.removeConnectionLocked(conn)
			break
		}
	}
	if result.Found {
		r.notifyLocked()
	}
	r.mu.Unlock()
	if target != nil {
		_ = target.Close()
	}
	return result
}

func (r *Registry) DisconnectPublisher(room string, port uint32) admin.ActionResult {
	key := roomPort{room: normalizeRoom(room), port: port}
	result := admin.ActionResult{Target: fmt.Sprintf("%s:%d", key.room, port)}
	r.mu.Lock()
	publisher := r.publishers[key]
	if publisher != nil {
		result.Found = true
		if publisher.Conn == nil {
			delete(r.publishers, key)
			result.PublishersRemoved = 1
		} else {
			result.ConnectionsClosed = 1
			result.PublishersRemoved = r.removeConnectionLocked(publisher.Conn)
		}
		r.notifyLocked()
	}
	r.mu.Unlock()
	if publisher != nil && publisher.Conn != nil {
		_ = publisher.Conn.Close()
	}
	return result
}

func closeConnections(conns []*ssh.ServerConn) {
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func (r *Registry) Snapshot() admin.Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rooms := make(map[string]*admin.Room)
	publishedPorts := make(map[*ssh.ServerConn][]uint32)
	for _, publisher := range r.publishers {
		publishedPorts[publisher.Conn] = append(publishedPorts[publisher.Conn], publisher.Port)
	}
	for _, ports := range publishedPorts {
		sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })
	}

	for conn, record := range r.connections {
		room := snapshotRoom(rooms, record.Room)
		room.ConnectionCount++
		room.ActiveChannels += record.ActiveChannels
		ports := append([]uint32(nil), publishedPorts[conn]...)
		room.Connections = append(room.Connections, admin.Connection{
			ID: record.ID, Room: record.Room, RemoteAddr: record.RemoteAddr,
			ConnectedAt: record.ConnectedAt, ActiveChannels: record.ActiveChannels,
			Role: connectionRole(record, len(ports) > 0), PublishedPorts: ports,
			ReceiverActiveChannels:  record.ReceiverActiveChannels,
			PublisherActiveChannels: record.PublisherActiveChannels,
		})
	}
	for _, publisher := range r.publishers {
		room := snapshotRoom(rooms, publisher.Room)
		room.PublisherCount++
		record := r.connections[publisher.Conn]
		var id uint64
		remote := ""
		if record != nil {
			id, remote = record.ID, record.RemoteAddr
		}
		room.Publishers = append(room.Publishers, admin.Publisher{
			Room: publisher.Room, BindHost: publisher.BindHost, Port: publisher.Port,
			ConnectionID: id, RemoteAddr: remote, RegisteredAt: publisher.RegisteredAt,
			ActiveChannels: publisher.ActiveChannels,
		})
	}

	out := admin.Snapshot{CapturedAt: time.Now().UTC(), Rooms: make([]admin.Room, 0, len(rooms))}
	for _, room := range rooms {
		sort.Slice(room.Connections, func(i, j int) bool { return room.Connections[i].ID < room.Connections[j].ID })
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
	sort.Slice(out.Rooms, func(i, j int) bool { return out.Rooms[i].Name < out.Rooms[j].Name })
	out.Totals.Rooms = len(out.Rooms)
	return out
}

func snapshotRoom(rooms map[string]*admin.Room, name string) *admin.Room {
	room := rooms[name]
	if room == nil {
		room = &admin.Room{Name: name, Connections: []admin.Connection{}, Publishers: []admin.Publisher{}}
		rooms[name] = room
	}
	return room
}

func normalizeRoom(room string) string { return strings.TrimSpace(room) }

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
