package admin

import "golang.org/x/crypto/ssh"

type Store interface {
	Snapshot(publicDomain, publicSSHPort, adminUser string, adminDashboardPort uint32) OverviewSnapshot
	DeleteRoom(room string) ([]*ssh.ServerConn, int)
	DeleteConnection(id uint64, protectedRoom string) (*ConnectionDeleteResult, bool, bool)
}

type OverviewSnapshot struct {
	PublicDomain       string         `json:"publicDomain"`
	PublicSSHPort      string         `json:"publicSshPort"`
	AdminUser          string         `json:"adminUser"`
	AdminDashboardPort uint32         `json:"adminDashboardPort"`
	Totals             OverviewTotals `json:"totals"`
	Rooms              []RoomSnapshot `json:"rooms"`
}

type OverviewTotals struct {
	Rooms          int `json:"rooms"`
	Connections    int `json:"connections"`
	Publishers     int `json:"publishers"`
	ActiveChannels int `json:"activeChannels"`
}

type RoomSnapshot struct {
	Name            string               `json:"name"`
	ConnectionCount int                  `json:"connectionCount"`
	PublisherCount  int                  `json:"publisherCount"`
	ActiveChannels  int                  `json:"activeChannels"`
	Connections     []ConnectionSnapshot `json:"connections"`
	Publishers      []PublisherSnapshot  `json:"publishers"`
}

type ConnectionSnapshot struct {
	ID                      uint64   `json:"id"`
	Room                    string   `json:"room"`
	RemoteAddr              string   `json:"remoteAddr"`
	ConnectedAt             string   `json:"connectedAt"`
	ActiveChannels          int      `json:"activeChannels"`
	Role                    string   `json:"role"`
	PublishedPorts          []uint32 `json:"publishedPorts"`
	ReceiverActiveChannels  int      `json:"receiverActiveChannels"`
	PublisherActiveChannels int      `json:"publisherActiveChannels"`
}

type PublisherSnapshot struct {
	BindHost     string `json:"bindHost"`
	Port         uint32 `json:"port"`
	ConnectionID uint64 `json:"connectionId"`
	RemoteAddr   string `json:"remoteAddr"`
	RegisteredAt string `json:"registeredAt"`
}

type ConnectionDeleteResult struct {
	Conn              *ssh.ServerConn
	ID                uint64
	Room              string
	RemovedPublishers int
}
