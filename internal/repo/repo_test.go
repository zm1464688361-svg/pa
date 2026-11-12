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

// TestDropLegacyNotifyIndex 幂等键迁移兼容：旧全局唯一索引应被移除，重复执行幂等（缺陷-9/10）。
func TestDropLegacyNotifyIndex(t *testing.T) {
	dsn := testDSN(t)
	db, err := Open(dsn, true)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	legacy := []struct{ table, index string }{
		{"payment_records", "idx_payment_records_notify_id"},
		{"abnormal_payments", "idx_abnormal_payments_channel_trade_no"},
	}
	// 先重建旧索引，再清理：确保"存在→执行删除"分支被覆盖（缺陷-10）
	for _, lg := range legacy {
		_ = db.Exec("CREATE UNIQUE INDEX " + lg.index + " ON " + lg.table + " (id)").Error
		if err := dropIndexIfPresent(db, lg.table, lg.index); err != nil {
			t.Fatalf("重建后清理 %s.%s 失败: %v", lg.table, lg.index, err)
		}
	}
	// 两个兼容包装函数均需可直接覆盖
	for i := 0; i < 2; i++ {
		if err := dropLegacyNotifyIndex(db); err != nil {
			t.Fatalf("第 %d 次清理通知旧索引失败: %v", i+1, err)
		}
		if err := dropLegacyAbnormalTradeIndex(db); err != nil {
			t.Fatalf("第 %d 次清理台账旧索引失败: %v", i+1, err)
		}
	}
	for _, lg := range legacy {
		for i := 0; i < 2; i++ {
			if err := dropIndexIfPresent(db, lg.table, lg.index); err != nil {
				t.Fatalf("第 %d 次清理 %s.%s 失败: %v", i+1, lg.table, lg.index, err)
			}
		}
		var n int64
		if err := db.Raw(
			"SELECT COUNT(*) FROM information_schema.statistics "+
				"WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?",
			lg.table, lg.index).Scan(&n).Error; err != nil {
			t.Fatalf("查询索引 %s: %v", lg.index, err)
		}
		if n != 0 {
			t.Fatalf("旧唯一索引 %s.%s 应已移除", lg.table, lg.index)
		}
	}
}
