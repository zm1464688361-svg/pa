package clock

import (
	"context"
	"os"
	"testing"

	"github.com/zm1464688361-svg/pa/internal/config"
	"github.com/zm1464688361-svg/pa/internal/repo"
)

func TestDBStoreRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = config.LoadEnvKey("test_dsn")
	}
	if dsn == "" {
		t.Skip("未配置测试数据库")
	}
	db, err := repo.Open(dsn, true)
	if err != nil {
		t.Skipf("测试库不可用: %v", err)
	}
	if err := repo.EnsureClockRow(context.Background(), db); err != nil {
		t.Fatalf("ensure row: %v", err)
	}
	s := &DBStore{DB: db}
	if err := s.SaveOffset(context.Background(), 123); err != nil {
		t.Fatalf("save: %v", err)
	}
	off, err := s.GetOffset(context.Background())
	if err != nil || off != 123 {
		t.Fatalf("get: %v %d", err, off)
	}
	s.SaveOffset(context.Background(), 0)
}
