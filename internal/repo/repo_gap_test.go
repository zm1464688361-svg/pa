package repo

import (
	"context"
	"testing"
)

// TestOpenRejectsBadDSN 非法 DSN 必须立即报错，不得返回可用连接。
func TestOpenRejectsBadDSN(t *testing.T) {
	if _, err := Open("这不是合法的 DSN", false); err == nil {
		t.Fatal("非法 DSN 应返回错误")
	}
}

// TestOpenRejectsUnreachableDB 指向不存在的库时连接校验应失败。
func TestOpenRejectsUnreachableDB(t *testing.T) {
	dsn := testDSN(t)
	// 把库名换成不存在的库：gorm.Open 的连接校验（ping）应报错
	broken := replaceDatabase(dsn, "petshop_db_not_exists_for_test")
	if _, err := Open(broken, false); err == nil {
		t.Fatal("不存在的数据库应返回错误")
	}
}

// TestClosedDBErrors 连接关闭后：
//   - migrateSchema 取迁移锁失败 → 返回错误（不静默继续迁移）
//   - WriteAudit 仅记录日志、不 panic（审计失败不影响主流程）
//   - EnsureClockRow 返回错误供调用方判断
func TestClosedDBErrors(t *testing.T) {
	dsn := testDSN(t)
	db, err := Open(dsn, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if err := migrateSchema(db); err == nil {
		t.Fatal("连接已关闭时迁移应返回错误")
	}
	// 审计写入失败只记日志，不 panic、不阻断调用方
	WriteAudit(db, 1, "owner", "unit_test_closed", 0, "closed db")
	if err := EnsureClockRow(context.Background(), db); err == nil {
		t.Fatal("连接已关闭时时钟行初始化应返回错误")
	}
}

// replaceDatabase 替换 DSN 中的库名（形如 user:pass@tcp(host:port)/db?params）。
func replaceDatabase(dsn, database string) string {
	at := -1
	for i := 0; i < len(dsn); i++ {
		if dsn[i] == '/' && i > 0 && dsn[i-1] == ')' {
			at = i
			break
		}
	}
	if at < 0 {
		return dsn
	}
	end := len(dsn)
	for i := at + 1; i < len(dsn); i++ {
		if dsn[i] == '?' {
			end = i
			break
		}
	}
	return dsn[:at+1] + database + dsn[end:]
}
