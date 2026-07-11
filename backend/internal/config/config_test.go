package config

import "testing"

func TestMustLoadAdminConfiguration(t *testing.T) {
	t.Setenv("SSH_PORT", "2200")
	t.Setenv("SSH_LISTEN_ADDR", "")
	t.Setenv("ADMIN_USER", "operator")
	t.Setenv("ADMIN_AUTHORIZED_KEYS_FILE", "data/operators")
	cfg := MustLoad()
	if cfg.SSHListenAddr != ":2200" || cfg.AdminUser != "operator" || cfg.AdminAuthorizedKeysFile != "data/operators" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestMustLoadAdminDefaults(t *testing.T) {
	t.Setenv("ADMIN_USER", "")
	t.Setenv("ADMIN_AUTHORIZED_KEYS_FILE", "")
	cfg := MustLoad()
	if cfg.AdminUser != "root" || cfg.AdminAuthorizedKeysFile != "" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}
