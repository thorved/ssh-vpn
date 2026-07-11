package tunnel

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/thorved/ssh-vpn/backend/internal/config"
	"github.com/thorved/ssh-vpn/backend/internal/sshauth"
	"golang.org/x/crypto/ssh"
)

func TestAdminAuthRequiresConfiguredPublicKey(t *testing.T) {
	allowed := testPublicKey(t)
	server := &Server{cfg: config.Config{AdminUser: "operator"}, adminKeys: testKeySet(t, allowed)}

	if _, err := server.noClientAuth(fakeMetadata{user: "room-a"}); err != nil {
		t.Fatalf("room auth failed: %v", err)
	}
	_, err := server.noClientAuth(fakeMetadata{user: "operator"})
	var partial *ssh.PartialSuccessError
	if !errors.As(err, &partial) {
		t.Fatalf("expected partial auth for admin, got %v", err)
	}
	if _, err := server.adminPublicKeyAuth(fakeMetadata{user: "operator"}, allowed); err != nil {
		t.Fatalf("allowed key rejected: %v", err)
	}
	if _, err := server.adminPublicKeyAuth(fakeMetadata{user: "operator"}, testPublicKey(t)); err == nil {
		t.Fatal("unknown key was accepted")
	}
}

func TestAdminAuthRejectsEmptyKeySet(t *testing.T) {
	keys, _ := sshauth.LoadAuthorizedKeys("")
	server := &Server{cfg: config.Config{AdminUser: "root"}, adminKeys: keys}
	if _, err := server.adminPublicKeyAuth(fakeMetadata{user: "root"}, testPublicKey(t)); err == nil {
		t.Fatal("expected rejection")
	}
}

func testPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func testKeySet(t *testing.T, keys ...ssh.PublicKey) *sshauth.AuthorizedKeys {
	t.Helper()
	path := filepath.Join(t.TempDir(), "authorized_keys")
	var data []byte
	for _, key := range keys {
		data = append(data, ssh.MarshalAuthorizedKey(key)...)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	set, err := sshauth.LoadAuthorizedKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

type fakeMetadata struct{ user string }

func (f fakeMetadata) User() string        { return f.user }
func (fakeMetadata) SessionID() []byte     { return nil }
func (fakeMetadata) ClientVersion() []byte { return nil }
func (fakeMetadata) ServerVersion() []byte { return nil }
func (fakeMetadata) RemoteAddr() net.Addr  { return nil }
func (fakeMetadata) LocalAddr() net.Addr   { return nil }
