// Package clock 提供教学时钟：所有业务时间判断必须走 Clock.Now()，
// 通过调整偏移量可把"当前时间"拨到任意时刻（演示 2 小时边界、5 分钟超时、时段结束等）。
package clock

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"
)

// OffsetStore 时钟偏移持久化（DB 单行）。
type OffsetStore interface {
	GetOffset(ctx context.Context) (int64, error)
	SaveOffset(ctx context.Context, offsetSec int64) error
}

// Clock 线程安全的教学时钟。
type Clock struct {
	mu     sync.RWMutex
	offset time.Duration
	base   time.Time // 创建时的真实时间，Now = base + (真实流逝) + offset
	store  OffsetStore
}

// New 创建时钟并从 store 加载偏移；store 为 nil 时退化为真实时钟。
func New(store OffsetStore) *Clock {
	c := &Clock{base: time.Now(), store: store}
	if store != nil {
		if off, err := store.GetOffset(context.Background()); err == nil {
			c.offset = time.Duration(off) * time.Second
		}
	}
	return c
}

// Now 返回当前教学时间。
func (c *Clock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Now().Add(c.offset)
}

// Offset 返回当前偏移秒数。
func (c *Clock) Offset() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return int64(c.offset.Seconds())
}

// SetOffset 调整偏移并持久化。offsetSec<=0 恢复真实时间。
func (c *Clock) SetOffset(offsetSec int64) error {
	c.mu.Lock()
	c.offset = time.Duration(offsetSec) * time.Second
	c.mu.Unlock()
	if c.store != nil {
		return c.store.SaveOffset(context.Background(), offsetSec)
	}
	return nil
}

// DBStore 基于 GORM 的偏移存储（表 clock_offsets 单行 id=1）。
type DBStore struct {
	DB *gorm.DB
}

func (s *DBStore) GetOffset(ctx context.Context) (int64, error) {
	var row struct {
		OffsetSec int64
	}
	err := s.DB.WithContext(ctx).Table("clock_offsets").
		Select("offset_sec").Where("id = ?", 1).Scan(&row).Error
	return row.OffsetSec, err
}

func (s *DBStore) SaveOffset(ctx context.Context, offsetSec int64) error {
	return s.DB.WithContext(ctx).Exec(
		"INSERT INTO clock_offsets (id, offset_sec) VALUES (1, ?) "+
			"ON DUPLICATE KEY UPDATE offset_sec = VALUES(offset_sec)", offsetSec).Error
}
