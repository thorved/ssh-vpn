package tunnel

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thorved/ssh-vpn/backend/internal/config"
	"github.com/thorved/ssh-vpn/backend/internal/sshauth"
	"golang.org/x/crypto/ssh"
)

func TestLoadAuthorizedKeysAllowsMultipleKeys(t *testing.T) {
	keyA := testPublicKey(t)
	keyB := testPublicKey(t)
	path := filepath.Join(t.TempDir(), "authorized_keys")
	content := "# admins\n" + string(ssh.MarshalAuthorizedKey(keyA)) + "\n" + string(ssh.MarshalAuthorizedKey(keyB))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write authorized keys: %v", err)
	}

	keys, err := sshauth.LoadAuthorizedKeys(path)
	if err != nil {
		t.Fatalf("load authorized keys: %v", err)
	}
	if !keys.Allows(keyA) {
		t.Fatal("expected first key to be allowed")
	}
	if !keys.Allows(keyB) {
		t.Fatal("expected second key to be allowed")
	}
}

func TestAdminAuthRequiresPublicKeyForAdminOnly(t *testing.T) {
	key := testPublicKey(t)
	keys := testAuthorizedKeys(t, key)
	server := &Server{
		cfg:       config.Config{AdminUser: "root"},
		adminKeys: keys,
	}

	perms, err := server.noClientAuth(fakeConnMetadata{user: "room-a"})
	if err != nil {
		t.Fatalf("non-admin no-client auth failed: %v", err)
	}
	if perms != nil {
		t.Fatalf("expected empty permissions for non-admin, got %#v", perms)
	}

	_, err = server.noClientAuth(fakeConnMetadata{user: "root"})
	var partial *ssh.PartialSuccessError
	if !errors.As(err, &partial) {
		t.Fatalf("expected admin to require public key partial success, got %v", err)
	}
	if _, err := server.adminPublicKeyAuth(fakeConnMetadata{user: "root"}, key); err != nil {
		t.Fatalf("admin key auth failed: %v", err)
	}
	if _, err := server.adminPublicKeyAuth(fakeConnMetadata{user: "root"}, testPublicKey(t)); err == nil {
		t.Fatal("expected unknown admin key to be rejected")
	}
}

func testPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()

	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatalf("convert public key: %v", err)
	}
	return key
}

func testAuthorizedKeys(t *testing.T, keys ...ssh.PublicKey) *sshauth.AuthorizedKeys {
	t.Helper()

	path := filepath.Join(t.TempDir(), "authorized_keys")
	var content strings.Builder
	for _, key := range keys {
		content.Write(ssh.MarshalAuthorizedKey(key))
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o600); err != nil {
		t.Fatalf("write authorized keys: %v", err)
	}
	authorizedKeys, err := sshauth.LoadAuthorizedKeys(path)
	if err != nil {
		t.Fatalf("load authorized keys: %v", err)
	}
	return authorizedKeys
}

type fakeConnMetadata struct {
	user string
}

func (f fakeConnMetadata) User() string {
	return f.user
}

func (f fakeConnMetadata) SessionID() []byte {
	return nil
}

func (f fakeConnMetadata) ClientVersion() []byte {
	return nil
}

func (f fakeConnMetadata) ServerVersion() []byte {
	return nil
}

func (f fakeConnMetadata) RemoteAddr() net.Addr {
	return nil
}

func (f fakeConnMetadata) LocalAddr() net.Addr {
	return nil
}
