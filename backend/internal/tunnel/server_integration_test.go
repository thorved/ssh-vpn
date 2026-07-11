package tunnel

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thorved/ssh-vpn/backend/internal/config"
	"golang.org/x/crypto/ssh"
)

func TestServerAdminOpensTerminalDashboard(t *testing.T) {
	adminSigner := testSigner(t)
	keysPath := filepath.Join(t.TempDir(), "authorized_keys")
	if err := os.WriteFile(keysPath, ssh.MarshalAuthorizedKey(adminSigner.PublicKey()), 0o600); err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(config.Config{SSHListenAddr: "127.0.0.1:0", SSHServerIdent: "SSH-2.0-test", AdminUser: "operator", AdminAuthorizedKeysFile: keysPath})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Run() }()
	t.Cleanup(func() { _ = server.Shutdown(); <-done })
	addr := waitForListener(t, server)

	roomClient, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{User: "room-a", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("ordinary room login failed: %v", err)
	}
	_ = roomClient.Close()
	deadline := time.Now().Add(time.Second)
	for server.registry.Snapshot().Totals.Connections != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	unauthorized := testSigner(t)
	if client, err := ssh.Dial("tcp", addr, clientConfig("operator", unauthorized)); err == nil {
		_ = client.Close()
		t.Fatal("unauthorized admin key was accepted")
	}

	client, err := ssh.Dial("tcp", addr, clientConfig("operator", adminSigner))
	if err != nil {
		t.Fatalf("admin login failed: %v", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var output lockedBuffer
	session.Stdout = &output
	session.Stderr = &output
	if err := session.RequestPty("xterm-256color", 30, 110, ssh.TerminalModes{}); err != nil {
		t.Fatalf("request pty: %v", err)
	}
	if err := session.Shell(); err != nil {
		t.Fatalf("start dashboard: %v", err)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), "SSH VPN") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(output.String(), "SSH VPN") {
		t.Fatalf("dashboard did not render; output=%q", output.String())
	}
	if !strings.Contains(output.String(), "\x1b[38;5;") {
		t.Fatal("SSH dashboard did not emit ANSI-256 colors")
	}
	if snapshot := server.registry.Snapshot(); snapshot.Totals.Connections != 0 || snapshot.Totals.Rooms != 0 {
		t.Fatalf("admin leaked into tunnel snapshot: %#v", snapshot.Totals)
	}
}

func TestDuplicatePublisherDisconnectsOnlyOffendingConnection(t *testing.T) {
	server, err := NewServer(config.Config{SSHListenAddr: "127.0.0.1:0", SSHServerIdent: "SSH-2.0-test", AdminUser: "root"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Run() }()
	t.Cleanup(func() { _ = server.Shutdown(); <-done })
	addr := waitForListener(t, server)
	roomConfig := &ssh.ClientConfig{User: "room-a", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 2 * time.Second}

	first, err := ssh.Dial("tcp", addr, roomConfig)
	if err != nil {
		t.Fatalf("dial first publisher: %v", err)
	}
	defer first.Close()
	firstListener, err := first.Listen("tcp", "localhost:48080")
	if err != nil {
		t.Fatalf("register first publisher: %v", err)
	}
	defer firstListener.Close()

	second, err := ssh.Dial("tcp", addr, roomConfig)
	if err != nil {
		t.Fatalf("dial duplicate publisher: %v", err)
	}
	defer second.Close()
	if duplicate, err := second.Listen("tcp", "localhost:48080"); err == nil {
		_ = duplicate.Close()
		t.Fatal("duplicate publisher was unexpectedly accepted")
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- second.Wait() }()
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("duplicate publisher connection remained open")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		totals := server.registry.Snapshot().Totals
		if totals.Connections == 1 && totals.Publishers == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("existing publisher was not preserved: %#v", server.registry.Snapshot().Totals)
}

func TestRoomUserOpensRoleDashboard(t *testing.T) {
	server, err := NewServer(config.Config{SSHListenAddr: "127.0.0.1:0", SSHServerIdent: "SSH-2.0-test", AdminUser: "root"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Run() }()
	t.Cleanup(func() { _ = server.Shutdown(); <-done })
	client, err := ssh.Dial("tcp", waitForListener(t, server), &ssh.ClientConfig{User: "room-a", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var output lockedBuffer
	session.Stdout, session.Stderr = &output, &output
	if err := session.RequestPty("xterm-256color", 30, 110, ssh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, &output, "SSH VPN", "room-a", "CONNECTED", "\x1b[38;5;")
}

func TestPublisherCanKickClientFromRoleDashboard(t *testing.T) {
	server, err := NewServer(config.Config{SSHListenAddr: "127.0.0.1:0", SSHServerIdent: "SSH-2.0-test", AdminUser: "root"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Run() }()
	t.Cleanup(func() { _ = server.Shutdown(); <-done })
	addr := waitForListener(t, server)
	roomConfig := &ssh.ClientConfig{User: "room-live", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 2 * time.Second}

	publisher, err := ssh.Dial("tcp", addr, roomConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	listener, err := publisher.Listen("tcp", "localhost:48082")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	pubSession, pubInput, pubOutput := openRoleSession(t, publisher)
	defer pubSession.Close()
	waitForOutput(t, pubOutput, "PUBLISHER", "48082")

	client, err := ssh.Dial("tcp", addr, roomConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	clientSession, clientInput, clientOutput := openRoleSession(t, client)
	defer clientSession.Close()

	accepted := make(chan net.Conn, 1)
	go func() { conn, _ := listener.Accept(); accepted <- conn }()
	clientTraffic, clientRequests, err := client.OpenChannel("direct-tcpip", ssh.Marshal(directTCPIPPayload{Host: "localhost", Port: 48082, OriginatorIP: "127.0.0.1", OriginatorPort: 52000}))
	if err != nil {
		t.Fatal(err)
	}
	go ssh.DiscardRequests(clientRequests)
	defer clientTraffic.Close()
	publisherTraffic := <-accepted
	if publisherTraffic == nil {
		t.Fatal("publisher did not receive forwarded traffic")
	}
	defer publisherTraffic.Close()

	_, _ = pubInput.Write([]byte("3"))
	_, _ = clientInput.Write([]byte("3"))
	waitForOutput(t, pubOutput, "FROM CLIENT")
	waitForOutput(t, clientOutput, "TO PUBLISHER")

	clientDone := make(chan error, 1)
	go func() { clientDone <- client.Wait() }()
	_, _ = pubInput.Write([]byte("d"))
	time.Sleep(30 * time.Millisecond)
	_, _ = pubInput.Write([]byte("y"))
	select {
	case <-clientDone:
	case <-time.After(3 * time.Second):
		t.Fatal("publisher kick did not close client SSH connection")
	}
}

func openRoleSession(t *testing.T, client *ssh.Client) (*ssh.Session, io.WriteCloser, *lockedBuffer) {
	t.Helper()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	input, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output := &lockedBuffer{}
	session.Stdout, session.Stderr = output, output
	if err := session.RequestPty("xterm-256color", 30, 110, ssh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	return session, input, output
}

func clientConfig(user string, signer ssh.Signer) *ssh.ClientConfig {
	return &ssh.ClientConfig{User: user, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 2 * time.Second}
}

func testSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func waitForListener(t *testing.T, server *Server) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		server.mu.Lock()
		listener := server.listener
		server.mu.Unlock()
		if listener != nil {
			return listener.Addr().String()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not start listening")
	return ""
}

func waitForOutput(t *testing.T, output *lockedBuffer, values ...string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		text := output.String()
		all := true
		for _, value := range values {
			if !strings.Contains(text, value) {
				all = false
				break
			}
		}
		if all {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("terminal output missing %q; output=%q", values, output.String())
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}
func (b *lockedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buffer.String() }
