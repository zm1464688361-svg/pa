package biz

import (
	"context"
	"time"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/repo"
	"gorm.io/gorm"
)

// MarkNoShow 店长标记未到店：
// 前置：时段【结束时间】必须已到达；预约未核销未取消（仍处已确认）；
// 原因必填；标记后状态不可回退；不自动退款、不自动返还名额。
func (b *Biz) MarkNoShow(ctx context.Context, actor *model.User, bookingID int64, reason string) (string, error) {
	if err := b.requireRole(actor.Role, "owner"); err != nil {
		return "", err
	}
	if reason == "" {
		return "", errs.ErrNoShowReason
	}
	msg := ""
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		ap, err := b.getBooking(tx, bookingID)
		if err != nil {
			return err
		}
		var s model.TimeSlot
		if err := tx.First(&s, ap.SlotID).Error; err != nil {
			return err
		}
		// 必须等时段结束时间到达之后
		if b.Clock.Now().Before(s.EndTime) {
			return errs.ErrNoShowNotEnded
		}
		if ap.Status == model.StatusNoShow {
			return errs.ErrNoShowDone
		}
		// 仅"时段结束仍未到店"（已确认且未核销）的预约可标记
		if ap.Status != model.StatusConfirmed {
			return errs.ErrStateInvalid.With("当前状态 %s 不可标记未到店", statusText(ap.Status))
		}
		now := b.Clock.Now()
		res := tx.Model(&model.Appointment{}).
			Where("id = ? AND status = ?", ap.ID, model.StatusConfirmed).
			Updates(map[string]interface{}{
				"status": model.StatusNoShow, "no_show_reason": reason, "no_show_at": now, "updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			msg = "已标记未到店（不自动退款、不返还名额）"
			repo.WriteAudit(tx, actor.ID, actor.Role, "mark_no_show", ap.ID, reason)
		} else {
			return errs.ErrStateInvalid
		}
		return nil
	})
	return msg, err
}

// UpdateNoShowReason 未到店原因可编辑（状态不可回退）。
func (b *Biz) UpdateNoShowReason(ctx context.Context, actor *model.User, bookingID int64, reason string) (string, error) {
	if err := b.requireRole(actor.Role, "owner"); err != nil {
		return "", err
	}
	if reason == "" {
		return "", errs.ErrNoShowReason
	}
	res := b.DB.Model(&model.Appointment{}).
		Where("id = ? AND status = ?", bookingID, model.StatusNoShow).
		Updates(map[string]interface{}{"no_show_reason": reason, "updated_at": time.Now()})
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", errs.ErrStateInvalid.With("仅未到店预约可编辑原因")
	}
	repo.WriteAudit(b.DB, actor.ID, actor.Role, "update_noshow_reason", bookingID, reason)
	return "未到店原因已更新", nil
}
