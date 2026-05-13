package tunnel

import (
	"errors"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestRegistryRegisterAndLookup(t *testing.T) {
	registry := NewRegistry()
	publisher := &Publisher{Room: "room-a", Port: 8080}

	if err := registry.Register(publisher); err != nil {
		t.Fatalf("register publisher: %v", err)
	}

	got, err := registry.Lookup("room-a", 8080)
	if err != nil {
		t.Fatalf("lookup publisher: %v", err)
	}
	if got != publisher {
		t.Fatal("lookup returned a different publisher")
	}
}

func TestRegistryRejectsDuplicateRoomPort(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register(&Publisher{Room: "room-a", Port: 8080}); err != nil {
		t.Fatalf("register first publisher: %v", err)
	}

	err := registry.Register(&Publisher{Room: "room-a", Port: 8080})
	if !errors.Is(err, ErrPublisherExists) {
		t.Fatalf("expected duplicate publisher error, got %v", err)
	}
}

func TestRegistryAllowsSamePortInDifferentRooms(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register(&Publisher{Room: "room-a", Port: 8080}); err != nil {
		t.Fatalf("register first publisher: %v", err)
	}
	if err := registry.Register(&Publisher{Room: "room-b", Port: 8080}); err != nil {
		t.Fatalf("register second publisher: %v", err)
	}
}

func TestRegistryUnregister(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register(&Publisher{Room: "room-a", Port: 8080}); err != nil {
		t.Fatalf("register publisher: %v", err)
	}
	if removed := registry.Unregister("room-a", 8080, nil); !removed {
		t.Fatal("expected unregister to remove publisher")
	}

	_, err := registry.Lookup("room-a", 8080)
	if !errors.Is(err, ErrPublisherNotFound) {
		t.Fatalf("expected missing publisher error, got %v", err)
	}
}

func TestRegistryUnregisterConn(t *testing.T) {
	registry := NewRegistry()
	conn := &ssh.ServerConn{}

	if err := registry.Register(&Publisher{Room: "room-a", Port: 8080, Conn: conn}); err != nil {
		t.Fatalf("register publisher: %v", err)
	}
	if removed := registry.UnregisterConn(conn); removed != 1 {
		t.Fatalf("expected 1 removed publisher, got %d", removed)
	}

	_, err := registry.Lookup("room-a", 8080)
	if !errors.Is(err, ErrPublisherNotFound) {
		t.Fatalf("expected missing publisher error, got %v", err)
	}
}

func TestRegistryLookupMissingPublisher(t *testing.T) {
	registry := NewRegistry()

	_, err := registry.Lookup("missing", 8080)
	if !errors.Is(err, ErrPublisherNotFound) {
		t.Fatalf("expected missing publisher error, got %v", err)
	}
}
