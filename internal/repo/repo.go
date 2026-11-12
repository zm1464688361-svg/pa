// Package repo 负责数据库连接、迁移与审计写入。
package repo

import (
	"context"
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
		if err := db.AutoMigrate(
			&model.User{}, &model.Shop{}, &model.ServiceItem{}, &model.Pet{},
			&model.TimeSlot{}, &model.Appointment{}, &model.PaymentRecord{},
			&model.Refund{}, &model.CancelRequest{}, &model.AbnormalPayment{},
			&model.AuditLog{}, &model.ClockOffset{},
		); err != nil {
			return nil, err
		}
	}
	return db, nil
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
