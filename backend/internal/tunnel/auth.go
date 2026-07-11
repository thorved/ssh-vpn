package tunnel

import (
	"errors"

	"golang.org/x/crypto/ssh"
)

func (s *Server) noClientAuth(conn ssh.ConnMetadata) (*ssh.Permissions, error) {
	if conn.User() != s.cfg.AdminUser {
		return nil, nil
	}
	return nil, &ssh.PartialSuccessError{Next: ssh.ServerAuthCallbacks{PublicKeyCallback: s.adminPublicKeyAuth}}
}

func (s *Server) adminPublicKeyAuth(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	if conn.User() != s.cfg.AdminUser {
		return nil, errors.New("public key auth is only enabled for admin")
	}
	if s.adminKeys.Empty() {
		return nil, errors.New("admin authorized keys file is not configured")
	}
	if !s.adminKeys.Allows(key) {
		return nil, errors.New("admin public key is not authorized")
	}
	return nil, nil
}
