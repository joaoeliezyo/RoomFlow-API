package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test@localhost/test")
	t.Setenv("APP_PORT", "")
	cfg, err := Load()
	if err != nil || cfg.Port != 8080 || cfg.DatabaseURL != "postgres://test@localhost/test" {
		t.Fatalf("Load() = %+v, %v", cfg, err)
	}
	t.Setenv("APP_PORT", "9090")
	cfg, err = Load()
	if err != nil || cfg.Port != 9090 {
		t.Fatalf("Load() with APP_PORT = %+v, %v", cfg, err)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing DATABASE_URL error")
	}
	t.Setenv("DATABASE_URL", "postgres://test@localhost/test")
	for _, value := range []string{"abc", "0", "65536"} {
		t.Setenv("APP_PORT", value)
		if _, err := Load(); err == nil {
			t.Fatalf("expected invalid APP_PORT error for %q", value)
		}
	}
}
