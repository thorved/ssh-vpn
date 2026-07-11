package userui

import "time"

type Store interface {
	Snapshot() Snapshot
	Subscribe() (<-chan struct{}, func())
	StopPublishing(port uint32) ActionResult
	CloseActivity(id uint64) ActionResult
	Disconnect() ActionResult
}

type Snapshot struct {
	CapturedAt   time.Time
	Connected    bool
	Room         string
	ConnectionID uint64
	RemoteAddr   string
	ConnectedAt  time.Time
	Role         string
	Published    []PublishedPort
	Activities   []Activity
}

type PublishedPort struct {
	Port          uint32
	BindHost      string
	RegisteredAt  time.Time
	ActiveClients int
}

type Activity struct {
	ID           uint64
	Direction    string
	Port         uint32
	PeerAddress  string
	ConnectedAt  time.Time
	ChannelCount int
}

type ActionResult struct {
	Found         bool
	Message       string
	Disconnecting bool
}
