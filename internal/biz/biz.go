// Package biz 实现题目 02 全部业务规则。所有时间判断走 Clock，所有状态变更加行锁。
package biz

import (
	"fmt"
	"math/rand"
	"time"

	signpkg "github.com/zm1464688361-svg/pa/internal/sign"

	"github.com/zm1464688361-svg/pa/internal/clock"
	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	payHoldDuration = 5 * time.Minute    // 待支付订单占用名额时长
	cancelCutoff    = 2 * time.Hour      // 自助取消提前量
	refundTimeout   = 7 * 24 * time.Hour // 退款回调超时阈值
	maxRefundRetry  = 3                  // 退款最大重试次数
)

// Biz 业务入口。
type Biz struct {
	DB    *gorm.DB
	Clock *clock.Clock
}

// ---------- 角色校验 ----------

func (b *Biz) requireRole(role string, allowed ...string) error {
	for _, a := range allowed {
		if role == a {
			return nil
		}
	}
	return errs.ErrForbidden
}

// ---------- 通用查询 ----------

func (b *Biz) getBooking(tx *gorm.DB, id int64) (*model.Appointment, error) {
	var ap model.Appointment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).First(&ap).Error; err != nil {
		return nil, errs.ErrNotFound.With("预约单 %d", id)
	}
	return &ap, nil
}

// locking 行锁表达式（SELECT ... FOR UPDATE）。
func locking() clause.Expression { return clause.Locking{Strength: "UPDATE"} }

func (b *Biz) getSlot(tx *gorm.DB, id int64) (*model.TimeSlot, error) {
	var s model.TimeSlot
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).First(&s).Error; err != nil {
		return nil, errs.ErrSlotNotFound
	}
	return &s, nil
}

// ---------- 惰性状态推进 ----------

// expireIfDue 待支付超时关单并释放名额（幂等：条件更新保证只执行一次）。
func (b *Biz) expireIfDue(tx *gorm.DB, ap *model.Appointment) bool {
	if ap.Status != model.StatusPendingPayment || b.Clock.Now().Before(ap.PayDeadline) {
		return false
	}
	res := tx.Model(&model.Appointment{}).
		Where("id = ? AND status = ?", ap.ID, model.StatusPendingPayment).
		Updates(map[string]interface{}{"status": model.StatusClosed, "updated_at": time.Now()})
	if res.Error == nil && res.RowsAffected == 1 {
		b.releaseSlot(tx, ap.SlotID)
		ap.Status = model.StatusClosed
		return true
	}
	return false
}

// releaseSlot 释放一个名额（幂等护栏：remaining < capacity 才自增）。
func (b *Biz) releaseSlot(tx *gorm.DB, slotID int64) {
	tx.Model(&model.TimeSlot{}).
		Where("id = ? AND remaining < capacity", slotID).
		UpdateColumn("remaining", gorm.Expr("remaining + 1"))
}

// releaseSlotOnce 退款成功后原路返还名额，仅一次（SlotReleased 标记幂等）。
func (b *Biz) releaseSlotOnce(tx *gorm.DB, ap *model.Appointment) {
	if ap.SlotReleased {
		return
	}
	res := tx.Model(&model.Appointment{}).
		Where("id = ? AND slot_released = ?", ap.ID, false).
		UpdateColumn("slot_released", true)
	if res.Error == nil && res.RowsAffected == 1 {
		b.releaseSlot(tx, ap.SlotID)
		ap.SlotReleased = true
	}
}

// expireDuePending 扫描全部到期未支付订单并关单（供列表/回调前调用）。
func (b *Biz) expireDuePending() {
	var due []model.Appointment
	b.DB.Where("status = ? AND pay_deadline <= ?", model.StatusPendingPayment, b.Clock.Now()).
		Find(&due)
	for i := range due {
		b.expireIfDue(b.DB, &due[i])
	}
}

// ---------- 生成器 ----------

func genOrderNo() string {
	return fmt.Sprintf("B%s%04d", time.Now().Format("20060102150405"), rand.Intn(10000))
}

func genRefundNo() string {
	return fmt.Sprintf("R%s%04d", time.Now().Format("20060102150405"), rand.Intn(10000))
}

func fmtTime(t time.Time) string { return t.Format("01-02 15:04") }

func fmtFull(t time.Time) string { return t.Format("2006-01-02 15:04:05") }

// slotRange 时段显示区间，如 "11-14 14:00 ~ 15:00"。
func slotRange(s *model.TimeSlot) string {
	return fmt.Sprintf("%s ~ %s", fmtTime(s.StartTime), s.EndTime.Format("15:04"))
}

// statusText 状态中文文案（与原型一致）。
func statusText(st string) string {
	switch st {
	case model.StatusPendingPayment:
		return "待支付"
	case model.StatusClosed:
		return "已关闭"
	case model.StatusConfirmed:
		return "已确认"
	case model.StatusCancelled:
		return "已取消"
	case model.StatusCheckedIn:
		return "已核销"
	case model.StatusInService:
		return "服务中"
	case model.StatusCompleted:
		return "已完成"
	case model.StatusNoShow:
		return "未到店"
	}
	return st
}

// checkSign 校验支付回调签名。
func checkPaySign(notifyID, orderNo, tradeNo string, amountFen int64, result, expect string) error {
	if expect == "" || !signpkg.Equal(signpkg.Pay(notifyID, orderNo, tradeNo, amountFen, result), expect) {
		return errs.ErrPaySign
	}
	return nil
}

// checkRefundSign 校验退款回调签名。
func checkRefundSign(notifyID, refundNo, result, expect string) error {
	if expect == "" || !signpkg.Equal(signpkg.Refund(notifyID, refundNo, result), expect) {
		return errs.ErrPaySign
	}
	return nil
}
