package sshauth

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

type AuthorizedKeys struct {
	keys map[string]struct{}
}

func LoadAuthorizedKeys(path string) (*AuthorizedKeys, error) {
	path = strings.TrimSpace(path)
	keys := &AuthorizedKeys{keys: make(map[string]struct{})}
	if path == "" {
		return keys, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read admin authorized keys: %w", err)
	}

	for len(data) > 0 {
		key, _, _, rest, err := ssh.ParseAuthorizedKey(data)
		if err != nil {
			line, remaining, _ := bytes.Cut(data, []byte("\n"))
			trimmed := strings.TrimSpace(string(line))
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				data = remaining
				continue
			}
			return nil, fmt.Errorf("parse admin authorized keys: %w", err)
		}
		keys.keys[string(key.Marshal())] = struct{}{}
		data = rest
	}

	return keys, nil
}

func (a *AuthorizedKeys) Allows(key ssh.PublicKey) bool {
	if a == nil || key == nil {
		return false
	}
	_, ok := a.keys[string(key.Marshal())]
	return ok
}

func (a *AuthorizedKeys) Empty() bool {
	return a == nil || len(a.keys) == 0
}
