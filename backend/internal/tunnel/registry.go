package tunnel

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

var (
	ErrPublisherExists   = errors.New("publisher already exists")
	ErrPublisherNotFound = errors.New("publisher not found")
)

type Registry struct {
	mu         sync.RWMutex
	publishers map[roomPort]*Publisher
}

type Publisher struct {
	Room     string
	BindHost string
	Port     uint32
	Conn     *ssh.ServerConn
}

type roomPort struct {
	room string
	port uint32
}

func NewRegistry() *Registry {
	return &Registry{
		publishers: make(map[roomPort]*Publisher),
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
	return removed
}

func normalizeRoom(room string) string {
	return strings.TrimSpace(room)
}
