package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	SSHListenAddr           string
	SSHHostKeyPath          string
	SSHServerIdent          string
	PublicDomain            string
	PublicSSHPort           string
	AdminUser               string
	AdminDashboardPort      uint32
	AdminAuthorizedKeysFile string
	WebStaticDir            string
}

func MustLoad() Config {
	return Config{
		SSHListenAddr:           sshListenAddr(),
		SSHHostKeyPath:          strings.TrimSpace(os.Getenv("SSH_HOST_KEY_PATH")),
		SSHServerIdent:          envOr("SSH_SERVER_IDENT", "SSH-2.0-ssh-vpn"),
		PublicDomain:            envOr("PUBLIC_DOMAIN", "localhost"),
		PublicSSHPort:           envOr("PUBLIC_SSH_PORT", envOr("SSH_PORT", "2222")),
		AdminUser:               envOr("ADMIN_USER", "root"),
		AdminDashboardPort:      envPort("ADMIN_DASHBOARD_PORT", 8080),
		AdminAuthorizedKeysFile: strings.TrimSpace(os.Getenv("ADMIN_AUTHORIZED_KEYS_FILE")),
		WebStaticDir:            envOr("WEB_STATIC_DIR", "frontend/out"),
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

func envPort(key string, fallback uint32) uint32 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	port, err := strconv.ParseUint(value, 10, 32)
	if err != nil || port == 0 || port > 65535 {
		return fallback
	}
	return uint32(port)
}
