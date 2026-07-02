package tunnel

import (
	"errors"
	"net"
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

func TestRegistrySnapshotIncludesConnectionsAndPublishers(t *testing.T) {
	registry := NewRegistry()
	conn := &ssh.ServerConn{}
	registry.RegisterConn("room-a", conn, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54820})

	if err := registry.Register(&Publisher{Room: "room-a", BindHost: "localhost", Port: 8080, Conn: conn}); err != nil {
		t.Fatalf("register publisher: %v", err)
	}
	done := registry.BeginForward(conn, conn)
	snapshot := registry.Snapshot("example.com", "2222", "root", 8080)
	done()

	if snapshot.Totals.Rooms != 1 {
		t.Fatalf("expected 1 room, got %d", snapshot.Totals.Rooms)
	}
	if snapshot.Totals.Connections != 1 {
		t.Fatalf("expected 1 connection, got %d", snapshot.Totals.Connections)
	}
	if snapshot.Totals.Publishers != 1 {
		t.Fatalf("expected 1 publisher, got %d", snapshot.Totals.Publishers)
	}
	if snapshot.Totals.ActiveChannels != 1 {
		t.Fatalf("expected 1 active channel, got %d", snapshot.Totals.ActiveChannels)
	}
	if snapshot.Rooms[0].Name != "room-a" {
		t.Fatalf("expected room-a, got %q", snapshot.Rooms[0].Name)
	}
	if snapshot.Rooms[0].Publishers[0].Port != 8080 {
		t.Fatalf("expected publisher port 8080, got %d", snapshot.Rooms[0].Publishers[0].Port)
	}
	if snapshot.Rooms[0].Connections[0].Role != "publisher+receiver" {
		t.Fatalf("expected publisher+receiver role, got %q", snapshot.Rooms[0].Connections[0].Role)
	}
	if len(snapshot.Rooms[0].Connections[0].PublishedPorts) != 1 || snapshot.Rooms[0].Connections[0].PublishedPorts[0] != 8080 {
		t.Fatalf("expected published port 8080, got %#v", snapshot.Rooms[0].Connections[0].PublishedPorts)
	}
}

func TestRegistrySnapshotUsesEmptySlices(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterConn("root", &ssh.ServerConn{}, nil)

	snapshot := registry.Snapshot("localhost", "2222", "root", 8080)
	if len(snapshot.Rooms) != 1 {
		t.Fatalf("expected 1 room, got %d", len(snapshot.Rooms))
	}
	if snapshot.Rooms[0].Connections == nil {
		t.Fatal("expected connections to be an empty array, got nil")
	}
	if snapshot.Rooms[0].Publishers == nil {
		t.Fatal("expected publishers to be an empty array, got nil")
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
	registry.RegisterConn("room-a", conn, nil)

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
	if snapshot := registry.Snapshot("localhost", "2222", "root", 8080); snapshot.Totals.Connections != 0 {
		t.Fatalf("expected connection cleanup, got %d connection(s)", snapshot.Totals.Connections)
	}
}

func TestRegistryLookupMissingPublisher(t *testing.T) {
	registry := NewRegistry()

	_, err := registry.Lookup("missing", 8080)
	if !errors.Is(err, ErrPublisherNotFound) {
		t.Fatalf("expected missing publisher error, got %v", err)
	}
}

func TestRegistryDeleteRoomRemovesConnectionsAndPublishers(t *testing.T) {
	registry := NewRegistry()
	roomConn := &ssh.ServerConn{}
	otherConn := &ssh.ServerConn{}
	registry.RegisterConn("room-a", roomConn, nil)
	registry.RegisterConn("room-b", otherConn, nil)

	if err := registry.Register(&Publisher{Room: "room-a", Port: 8080, Conn: roomConn}); err != nil {
		t.Fatalf("register room-a publisher: %v", err)
	}
	if err := registry.Register(&Publisher{Room: "room-b", Port: 8080, Conn: otherConn}); err != nil {
		t.Fatalf("register room-b publisher: %v", err)
	}

	conns, publishers := registry.DeleteRoom("room-a")
	if len(conns) != 1 {
		t.Fatalf("expected 1 closed connection, got %d", len(conns))
	}
	if publishers != 1 {
		t.Fatalf("expected 1 removed publisher, got %d", publishers)
	}

	if _, err := registry.Lookup("room-a", 8080); !errors.Is(err, ErrPublisherNotFound) {
		t.Fatalf("expected room-a publisher removed, got %v", err)
	}
	if _, err := registry.Lookup("room-b", 8080); err != nil {
		t.Fatalf("expected room-b publisher to remain: %v", err)
	}
}

func TestRegistryDeleteConnectionRemovesOnlyThatConnection(t *testing.T) {
	registry := NewRegistry()
	roomConn := &ssh.ServerConn{}
	otherConn := &ssh.ServerConn{}
	registry.RegisterConn("room-a", roomConn, nil)
	registry.RegisterConn("room-a", otherConn, nil)

	if err := registry.Register(&Publisher{Room: "room-a", Port: 8080, Conn: roomConn}); err != nil {
		t.Fatalf("register publisher: %v", err)
	}
	snapshot := registry.Snapshot("localhost", "2222", "root", 8080)
	id := snapshot.Rooms[0].Connections[0].ID
	if snapshot.Rooms[0].Connections[0].PublishedPorts == nil || len(snapshot.Rooms[0].Connections[0].PublishedPorts) == 0 {
		id = snapshot.Rooms[0].Connections[1].ID
	}

	result, exists, protected := registry.DeleteConnection(id, "root")
	if !exists {
		t.Fatal("expected connection to exist")
	}
	if protected {
		t.Fatal("expected room-a connection not to be protected")
	}
	if result.RemovedPublishers != 1 {
		t.Fatalf("expected 1 removed publisher, got %d", result.RemovedPublishers)
	}
	if _, err := registry.Lookup("room-a", 8080); !errors.Is(err, ErrPublisherNotFound) {
		t.Fatalf("expected publisher removed, got %v", err)
	}
	if snapshot := registry.Snapshot("localhost", "2222", "root", 8080); snapshot.Totals.Connections != 1 {
		t.Fatalf("expected one connection to remain, got %d", snapshot.Totals.Connections)
	}
}

func TestRegistryDeleteConnectionProtectsAdminRoom(t *testing.T) {
	registry := NewRegistry()
	conn := &ssh.ServerConn{}
	registry.RegisterConn("root", conn, nil)
	id := registry.Snapshot("localhost", "2222", "root", 8080).Rooms[0].Connections[0].ID

	_, exists, protected := registry.DeleteConnection(id, "root")
	if !exists {
		t.Fatal("expected connection to exist")
	}
	if !protected {
		t.Fatal("expected admin connection to be protected")
	}
	if snapshot := registry.Snapshot("localhost", "2222", "root", 8080); snapshot.Totals.Connections != 1 {
		t.Fatalf("expected protected connection to remain, got %d", snapshot.Totals.Connections)
	}
}
