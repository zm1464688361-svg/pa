// Package repo 负责数据库连接、迁移与审计写入。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"
	_ "time/tzdata" // Windows 下内嵌时区数据，保证 loc=Local 可用

	"github.com/zm1464688361-svg/pa/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 连接 MySQL 并自动迁移。DSN 形如
// root:password@tcp(127.0.0.1:3306)/petshop?charset=utf8mb4&parseTime=True&loc=Local
func Open(dsn string, migrate bool) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if migrate {
		if err := migrateSchema(db); err != nil {
			return nil, err
		}
	}
	return db, nil
}

// migrateSchema 执行 AutoMigrate，并用 MySQL 命名锁串行化。
// 多包并行测试（go test ./internal/... 会用多个进程同时连接同一测试库）时，
// 并发 AutoMigrate 可能因表已存在（1050）互相冲突，命名锁保证同一时刻只有一个迁移者。
func migrateSchema(db *gorm.DB) error {
	const lockName = "petshop_schema_migrate"
	var got sql.NullInt64
	if err := db.Raw("SELECT GET_LOCK(?, 30)", lockName).Scan(&got).Error; err != nil {
		return err
	}
	if !got.Valid || got.Int64 != 1 {
		return fmt.Errorf("获取迁移锁超时: %s", lockName)
	}
	defer db.Exec("SELECT RELEASE_LOCK(?)", lockName)
	return errors.Join(
		db.AutoMigrate(
			&model.User{}, &model.Shop{}, &model.ServiceItem{}, &model.Pet{},
			&model.TimeSlot{}, &model.Appointment{}, &model.PaymentRecord{},
			&model.Refund{}, &model.CancelRequest{}, &model.AbnormalPayment{},
			&model.AuditLog{}, &model.ClockOffset{},
		),
		dropLegacyNotifyIndex(db),
		dropLegacyAbnormalTradeIndex(db),
	)
}

// dropLegacyNotifyIndex 兼容旧库：幂等键已改为 (order_no, notify_id) 复合唯一，
// 旧的全局 notify_id 唯一索引会让渠道重启后复用通知号的新支付写入失败，需移除。
func dropLegacyNotifyIndex(db *gorm.DB) error {
	return dropIndexIfPresent(db, "payment_records", "idx_payment_records_notify_id")
}

// dropLegacyAbnormalTradeIndex 兼容旧库：异常台账幂等键已改为 (order_no, channel_trade_no) 复合唯一，
// 旧的全局 channel_trade_no 唯一索引会让渠道重启后复用交易号的迟到支付无法登记台账，需移除。
func dropLegacyAbnormalTradeIndex(db *gorm.DB) error {
	return dropIndexIfPresent(db, "abnormal_payments", "idx_abnormal_payments_channel_trade_no")
}

// dropIndexIfPresent 按索引名存在与否决定是否删除，保证迁移幂等。
func dropIndexIfPresent(db *gorm.DB, table, index string) error {
	var n int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM information_schema.statistics "+
			"WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?",
		table, index).Scan(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	return db.Exec("ALTER TABLE " + table + " DROP INDEX " + index).Error
}

// WriteAudit 写审计日志，失败仅记日志不影响主流程。
func WriteAudit(db *gorm.DB, actorID int64, role, action string, targetID int64, detail string) {
	al := model.AuditLog{
		ActorID: actorID, Role: role, Action: action, TargetID: targetID,
		Detail: detail, CreatedAt: time.Now(),
	}
	if err := db.Create(&al).Error; err != nil {
		log.Printf("audit write failed: %v", err)
	}
}

// EnsureClockRow 保证 clock_offsets 单行存在。
func EnsureClockRow(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Exec(
		"INSERT IGNORE INTO clock_offsets (id, offset_sec, updated_at) VALUES (1, 0, NOW())").Error
}
