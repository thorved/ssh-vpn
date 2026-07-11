package sshauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestLoadAuthorizedKeys(t *testing.T) {
	keyA := publicKey(t)
	keyB := publicKey(t)
	path := filepath.Join(t.TempDir(), "authorized_keys")
	content := "# operators\n\n" + string(ssh.MarshalAuthorizedKey(keyA)) + string(ssh.MarshalAuthorizedKey(keyB))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	keys, err := LoadAuthorizedKeys(path)
	if err != nil {
		t.Fatalf("load keys: %v", err)
	}
	if keys.Empty() || !keys.Allows(keyA) || !keys.Allows(keyB) {
		t.Fatal("expected both configured keys to be allowed")
	}
	if keys.Allows(publicKey(t)) {
		t.Fatal("unexpected key was allowed")
	}
}

func TestLoadAuthorizedKeysEmptyPath(t *testing.T) {
	keys, err := LoadAuthorizedKeys("")
	if err != nil {
		t.Fatal(err)
	}
	if !keys.Empty() {
		t.Fatal("expected empty key set")
	}
}

func TestLoadAuthorizedKeysErrors(t *testing.T) {
	if _, err := LoadAuthorizedKeys(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing file error")
	}
	path := filepath.Join(t.TempDir(), "authorized_keys")
	if err := os.WriteFile(path, []byte("not a key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthorizedKeys(path); err == nil {
		t.Fatal("expected invalid key error")
	}
}

func publicKey(t *testing.T) ssh.PublicKey {
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
