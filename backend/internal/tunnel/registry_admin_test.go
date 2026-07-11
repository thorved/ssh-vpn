package tunnel

import (
	"errors"
	"net"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestRegistrySnapshotAndEvents(t *testing.T) {
	r := NewRegistry()
	events, cancel := r.Subscribe()
	defer cancel()
	publisherConn := testServerConn()
	receiverConn := testServerConn()
	r.RegisterConn("room-b", publisherConn, testAddr("10.0.0.1:1000"))
	r.RegisterConn("room-a", receiverConn, testAddr("10.0.0.2:2000"))
	publisher := &Publisher{Room: "room-b", BindHost: "localhost", Port: 8080, Conn: publisherConn}
	if err := r.Register(publisher); err != nil {
		t.Fatal(err)
	}
	done := r.BeginForward(receiverConn, publisher)

	snapshot := r.Snapshot()
	if snapshot.Totals.Rooms != 2 || snapshot.Totals.Connections != 2 || snapshot.Totals.Publishers != 1 || snapshot.Totals.ActiveChannels != 2 {
		t.Fatalf("unexpected totals: %#v", snapshot.Totals)
	}
	if snapshot.Rooms[0].Name != "room-a" || snapshot.Rooms[1].Name != "room-b" {
		t.Fatalf("rooms not sorted: %#v", snapshot.Rooms)
	}
	if snapshot.Rooms[1].Publishers[0].ActiveChannels != 1 {
		t.Fatal("publisher activity not tracked")
	}
	select {
	case <-events:
	default:
		t.Fatal("expected coalesced registry event")
	}
	done()
}

func TestRegistryManagementActions(t *testing.T) {
	r := NewRegistry()
	connA := testServerConn()
	connB := testServerConn()
	idA := r.RegisterConn("room-a", connA, nil)
	r.RegisterConn("room-a", connB, nil)
	if err := r.Register(&Publisher{Room: "room-a", Port: 8000, Conn: connA}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&Publisher{Room: "room-a", Port: 9000, Conn: connA}); err != nil {
		t.Fatal(err)
	}

	result := r.DisconnectPublisher("room-a", 8000)
	if !result.Found || result.ConnectionsClosed != 1 || result.PublishersRemoved != 2 {
		t.Fatalf("unexpected publisher result: %#v", result)
	}
	if !connA.Conn.(*fakeSSHConn).isClosed() {
		t.Fatal("publisher owner was not closed")
	}
	if again := r.DisconnectConnection(idA); again.Found {
		t.Fatal("removed connection found twice")
	}
	if snapshot := r.Snapshot(); snapshot.Totals.Connections != 1 {
		t.Fatalf("expected one remaining connection: %#v", snapshot.Totals)
	}

	roomResult := r.DeleteRoom("room-a")
	if !roomResult.Found || roomResult.ConnectionsClosed != 1 {
		t.Fatalf("unexpected room result: %#v", roomResult)
	}
	if r.DeleteRoom("room-a").Found {
		t.Fatal("removed room found twice")
	}
}

func TestRegistryDeleteRoomRemovesOrphanPublisher(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Publisher{Room: "orphan", Port: 8080}); err != nil {
		t.Fatal(err)
	}
	result := r.DeleteRoom("orphan")
	if !result.Found || result.PublishersRemoved != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if _, err := r.Lookup("orphan", 8080); !errors.Is(err, ErrPublisherNotFound) {
		t.Fatalf("publisher remained: %v", err)
	}
}

func TestRegistryConcurrentSnapshotAndMutation(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(port uint32) {
			defer wg.Done()
			conn := testServerConn()
			r.RegisterConn("room", conn, nil)
			_ = r.Register(&Publisher{Room: "room", Port: port, Conn: conn})
			_ = r.Snapshot()
			r.UnregisterConn(conn)
		}(uint32(1000 + i))
	}
	wg.Wait()
	if got := r.Snapshot().Totals; got.Connections != 0 || got.Publishers != 0 {
		t.Fatalf("registry did not clean up: %#v", got)
	}
}

type fakeSSHConn struct {
	mu     sync.Mutex
	closed bool
}

func (f *fakeSSHConn) User() string                                           { return "test" }
func (f *fakeSSHConn) SessionID() []byte                                      { return nil }
func (f *fakeSSHConn) ClientVersion() []byte                                  { return nil }
func (f *fakeSSHConn) ServerVersion() []byte                                  { return nil }
func (f *fakeSSHConn) RemoteAddr() net.Addr                                   { return testAddr("remote") }
func (f *fakeSSHConn) LocalAddr() net.Addr                                    { return testAddr("local") }
func (f *fakeSSHConn) SendRequest(string, bool, []byte) (bool, []byte, error) { return false, nil, nil }
func (f *fakeSSHConn) OpenChannel(string, []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, errors.New("not implemented")
}
func (f *fakeSSHConn) Close() error   { f.mu.Lock(); f.closed = true; f.mu.Unlock(); return nil }
func (f *fakeSSHConn) Wait() error    { return nil }
func (f *fakeSSHConn) isClosed() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.closed }
func testServerConn() *ssh.ServerConn { return &ssh.ServerConn{Conn: &fakeSSHConn{}} }

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

func TestTerminalSizeDefaults(t *testing.T) {
	w, h := terminalSize(0, 0)
	if w != 80 || h != 24 {
		t.Fatalf("unexpected default: %dx%d", w, h)
	}
	w, h = terminalSize(132, 43)
	if w != 132 || h != 43 {
		t.Fatalf("unexpected size: %dx%d", w, h)
	}
}
