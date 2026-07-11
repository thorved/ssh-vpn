package config

import (
	"os"
	"strings"
)

type Config struct {
	SSHListenAddr           string
	SSHHostKeyPath          string
	SSHServerIdent          string
	AdminUser               string
	AdminAuthorizedKeysFile string
}

func MustLoad() Config {
	return Config{
		SSHListenAddr:           sshListenAddr(),
		SSHHostKeyPath:          strings.TrimSpace(os.Getenv("SSH_HOST_KEY_PATH")),
		SSHServerIdent:          envOr("SSH_SERVER_IDENT", "SSH-2.0-ssh-vpn"),
		AdminUser:               envOr("ADMIN_USER", "root"),
		AdminAuthorizedKeysFile: strings.TrimSpace(os.Getenv("ADMIN_AUTHORIZED_KEYS_FILE")),
	}
}

func sshListenAddr() string {
	if value := strings.TrimSpace(os.Getenv("SSH_LISTEN_ADDR")); value != "" {
		return value
	}
	return ":" + envOr("SSH_PORT", "2222")
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
