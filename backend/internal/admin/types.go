package admin

import "time"

// Store is the live control-plane interface consumed by the SSH admin UI.
type Store interface {
	Snapshot() Snapshot
	Subscribe() (<-chan struct{}, func())
	DeleteRoom(room string) ActionResult
	DisconnectConnection(id uint64) ActionResult
	DisconnectPublisher(room string, port uint32) ActionResult
}

type Snapshot struct {
	CapturedAt time.Time
	Totals     Totals
	Rooms      []Room
}

type Totals struct {
	Rooms          int
	Connections    int
	Publishers     int
	ActiveChannels int
}

type Room struct {
	Name            string
	ConnectionCount int
	PublisherCount  int
	ActiveChannels  int
	Connections     []Connection
	Publishers      []Publisher
}

type Connection struct {
	ID                      uint64
	Room                    string
	RemoteAddr              string
	ConnectedAt             time.Time
	ActiveChannels          int
	Role                    string
	PublishedPorts          []uint32
	ReceiverActiveChannels  int
	PublisherActiveChannels int
}

type Publisher struct {
	Room           string
	BindHost       string
	Port           uint32
	ConnectionID   uint64
	RemoteAddr     string
	RegisteredAt   time.Time
	ActiveChannels int
}

type ActionResult struct {
	Found             bool
	Target            string
	ConnectionsClosed int
	PublishersRemoved int
}
