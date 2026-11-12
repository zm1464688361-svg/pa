package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadKeyAndUnquote(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.local.yaml")
	os.WriteFile(p, []byte("db_dsn: \"root:pw@tcp(h)/db?x=1\"\r\ntest_dsn: plain-value\r\n"), 0644)
	if v := readKey(p, "db_dsn"); v != "root:pw@tcp(h)/db?x=1" {
		t.Fatalf("双引号值: %q", v)
	}
	if v := readKey(p, "test_dsn"); v != "plain-value" {
		t.Fatalf("裸值: %q", v)
	}
	if v := readKey(p, "missing"); v != "" {
		t.Fatalf("缺失键应为空: %q", v)
	}
	// BOM 兼容
	os.WriteFile(p, []byte("\ufeffdb_dsn: 'single'\n"), 0644)
	if v := readKey(p, "db_dsn"); v != "single" {
		t.Fatalf("BOM+单引号: %q", v)
	}
}

func TestLoadEnvKeyMissingFile(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(old)
	if v := LoadEnvKey("db_dsn"); v != "" {
		t.Fatalf("无配置文件应为空: %q", v)
	}
}

func TestLoadDSNEnvPriority(t *testing.T) {
	old, ok := os.LookupEnv("DB_DSN")
	os.Setenv("DB_DSN", "env-dsn")
	defer func() {
		if ok {
			os.Setenv("DB_DSN", old)
		} else {
			os.Unsetenv("DB_DSN")
		}
	}()
	if LoadDSN() != "env-dsn" {
		t.Fatal("环境变量优先")
	}
}
