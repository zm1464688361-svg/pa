package biz

import (
	"context"
	"time"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/repo"
	"gorm.io/gorm"
)

// CheckIn 前台核销。前置校验（强制）：本店、已确认、无审核中取消申请。
// 幂等：重复核销返回成功提示，不产生新流水、不重复记收入。
func (b *Biz) CheckIn(ctx context.Context, actor *model.User, bookingID int64) (string, error) {
	if err := b.requireRole(actor.Role, "front"); err != nil {
		return "", err
	}
	msg := ""
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		ap, err := b.getBooking(tx, bookingID)
		if err != nil {
			return err
		}
		b.expireIfDue(tx, ap)
		// 跨店核销直接拒绝（账号绑定唯一门店）
		if ap.ShopID != actor.ShopID {
			return errs.ErrCheckInForbidden.With("预约门店 %d 与绑定门店 %d 不一致", ap.ShopID, actor.ShopID)
		}
		// 与取消并发：核销先落库 → 取消申请将被拒绝；取消申请已存在 → 核销拦截
		pending, err := b.HasPendingCancel(tx, ap.ID)
		if err != nil {
			return err
		}
		if pending {
			return errs.ErrCheckInForbidden.With("存在审核中的取消申请")
		}
		if ap.Status == model.StatusCheckedIn {
			msg = "重复核销：该预约已核销（幂等，不重复记收入）"
			return nil
		}
		if ap.Status != model.StatusConfirmed {
			return errs.ErrCheckInForbidden.With("当前状态 %s", statusText(ap.Status))
		}
		now := b.Clock.Now()
		res := tx.Model(&model.Appointment{}).
			Where("id = ? AND status = ?", ap.ID, model.StatusConfirmed).
			Updates(map[string]interface{}{"status": model.StatusCheckedIn, "checked_in_at": now, "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			msg = "核销成功，请登记服务开始"
			repo.WriteAudit(tx, actor.ID, actor.Role, "check_in", ap.ID, ap.OrderNo+" 核销，码 "+codeOf(ap))
		} else {
			msg = "重复核销：该预约已核销（幂等，不重复记收入）"
		}
		return nil
	})
	return msg, err
}

func codeOf(ap *model.Appointment) string { return ap.OrderNo }

// ServiceProgress 登记服务开始 / 完成。顺序强制：核销 → 开始 → 完成；三步全部幂等。
func (b *Biz) ServiceProgress(ctx context.Context, actor *model.User, bookingID int64, action string) (string, error) {
	if err := b.requireRole(actor.Role, "front"); err != nil {
		return "", err
	}
	msg := ""
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		ap, err := b.getBooking(tx, bookingID)
		if err != nil {
			return err
		}
		if ap.ShopID != actor.ShopID {
			return errs.ErrForbidden.With("非本店预约")
		}
		now := b.Clock.Now()
		switch action {
		case "start":
			switch ap.Status {
			case model.StatusInService:
				msg = "重复操作：服务已登记开始（幂等，不产生新流水）"
				return nil
			case model.StatusCheckedIn:
				res := tx.Model(&model.Appointment{}).
					Where("id = ? AND status = ?", ap.ID, model.StatusCheckedIn).
					Updates(map[string]interface{}{"status": model.StatusInService, "start_at": now, "updated_at": now})
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected == 1 {
					msg = "服务已登记开始"
					repo.WriteAudit(tx, actor.ID, actor.Role, "service_start", ap.ID, ap.OrderNo)
				} else {
					msg = "重复操作：服务已登记开始"
				}
				return nil
			default:
				return errs.ErrStartNotCheckedIn.With("当前状态 %s", statusText(ap.Status))
			}
		case "complete":
			switch ap.Status {
			case model.StatusCompleted:
				msg = "重复操作：服务已完成（幂等，不重复累加营收）"
				return nil
			case model.StatusInService:
				// 允许跨天登记完成
				res := tx.Model(&model.Appointment{}).
					Where("id = ? AND status = ?", ap.ID, model.StatusInService).
					Updates(map[string]interface{}{"status": model.StatusCompleted, "completed_at": now, "updated_at": now})
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected == 1 {
					msg = "服务已登记完成"
					repo.WriteAudit(tx, actor.ID, actor.Role, "service_complete", ap.ID, ap.OrderNo)
				} else {
					msg = "重复操作：服务已完成"
				}
				return nil
			default:
				return errs.ErrCompleteNotStarted.With("当前状态 %s", statusText(ap.Status))
			}
		default:
			return errs.ErrBadRequest.With("未知 action %s", action)
		}
	})
	return msg, err
}

// ListShopBookings 前台查看当日分配预约（本店）。
func (b *Biz) ListShopBookings(ctx context.Context, actor *model.User) ([]BookingRow, error) {
	if err := b.requireRole(actor.Role, "front"); err != nil {
		return nil, err
	}
	rows, err := b.ListBookings(ctx, actor, 0)
	return rows, err
}

var _ = time.Now
var _ = gorm.ErrRecordNotFound
