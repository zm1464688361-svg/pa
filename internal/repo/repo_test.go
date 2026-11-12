package repo

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/zm1464688361-svg/pa/internal/config"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = config.LoadEnvKey("test_dsn")
	}
	if dsn == "" {
		t.Skip("未配置测试数据库：设置环境变量 TEST_DB_DSN 或 config.local.yaml 的 test_dsn")
	}
	return dsn
}

// TestMigrateConcurrent 并发打开同一测试库：命名锁应保证 AutoMigrate 不互相冲突（1050）。
func TestMigrateConcurrent(t *testing.T) {
	dsn := testDSN(t)
	const n = 4
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := Open(dsn, true)
			if err != nil {
				errs[i] = err
				return
			}
			if sqlDB, e := db.DB(); e == nil {
				_ = sqlDB.Close()
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("第 %d 个并发迁移失败: %v", i, err)
		}
	}
}

// TestWriteAuditAndClockRow 审计写入与时钟单行初始化（重复调用应幂等）。
func TestWriteAuditAndClockRow(t *testing.T) {
	dsn := testDSN(t)
	db, err := Open(dsn, true)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := EnsureClockRow(ctx, db); err != nil {
			t.Fatalf("ensure clock row: %v", err)
		}
	}
	var cnt int64
	if err := db.Table("clock_offsets").Where("id = 1").Count(&cnt).Error; err != nil {
		t.Fatalf("count clock: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("clock_offsets 应只有 1 行，实际 %d", cnt)
	}
	WriteAudit(db, 0, "test", "unit_test", 0, "repo 单测写入")
	var n int64
	if err := db.Table("audit_logs").Where("action = ?", "unit_test").Count(&n).Error; err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if n == 0 {
		t.Fatal("审计日志未写入")
	}
	db.Exec("DELETE FROM audit_logs WHERE action = ?", "unit_test")
}
