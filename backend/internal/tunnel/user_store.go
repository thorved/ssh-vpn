package tunnel

import (
	"sync/atomic"

	"github.com/thorved/ssh-vpn/backend/internal/userui"
	"golang.org/x/crypto/ssh"
)

type userStore struct {
	registry   *Registry
	conn       *ssh.ServerConn
	disconnect atomic.Bool
}

func (s *userStore) Snapshot() userui.Snapshot            { return s.registry.UserSnapshot(s.conn) }
func (s *userStore) Subscribe() (<-chan struct{}, func()) { return s.registry.Subscribe() }
func (s *userStore) StopPublishing(port uint32) userui.ActionResult {
	return s.registry.StopPublishing(s.conn, port)
}
func (s *userStore) CloseActivity(id uint64) userui.ActionResult {
	return s.registry.CloseUserActivity(s.conn, id)
}
func (s *userStore) Disconnect() userui.ActionResult {
	s.disconnect.Store(true)
	return userui.ActionResult{Found: true, Message: "disconnecting SSH session", Disconnecting: true}
}

func (s *userStore) shouldDisconnect() bool { return s.disconnect.Load() }
