package biz

import (
	"context"
	"time"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/repo"
	"gorm.io/gorm"
)

// SubmitCancel 主人提交取消申请：
// 距开始 ≥2h：自助取消，立即发起全额退款（名额在退款成功后释放一次）；
// 距开始 <2h：原因必填，进入客服审核（名额锁定、禁止核销）。
// 与核销并发：以先落库为准——预约已核销时申请直接拒绝。
func (b *Biz) SubmitCancel(ctx context.Context, actor *model.User, bookingID int64, reason string) (*model.CancelRequest, string, error) {
	if err := b.requireRole(actor.Role, "pet_owner"); err != nil {
		return nil, "", err
	}
	if reason == "" {
		return nil, "", errs.ErrCancelReasonRequired
	}
	var cr *model.CancelRequest
	msg := ""
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		ap, err := b.getBooking(tx, bookingID)
		if err != nil {
			return err
		}
		if ap.UserID != actor.ID {
			return errs.ErrNotYourBooking
		}
		b.expireIfDue(tx, ap)
		// 分支互斥：已核销/服务中/已完成不能走取消退款
		if ap.Status != model.StatusConfirmed && ap.Status != model.StatusPendingPayment {
			return errs.ErrCancelNotCancellable.With("当前状态 %s", statusText(ap.Status))
		}
		if ap.Status == model.StatusPendingPayment {
			// 待支付单直接走主动取消（CancelUnpaid），此处明确提示
			return errs.ErrStateInvalid.With("待支付订单请使用主动取消")
		}
		// 已有待审核申请 → 幂等返回
		var existed model.CancelRequest
		if err := tx.Where("booking_id = ? AND status = ?", ap.ID, model.CancelPending).
			First(&existed).Error; err == nil {
			cr = &existed
			msg = "已存在待审核的取消申请，请等待客服处理"
			return nil
		}
		// 时段信息计算 2 小时边界
		var s model.TimeSlot
		if err := tx.First(&s, ap.SlotID).Error; err != nil {
			return err
		}
		urgent := s.StartTime.Sub(b.Clock.Now()) < cancelCutoff
		now := time.Now()
		if !urgent {
			// 自助取消：申请直接生效，发起退款
			cr = &model.CancelRequest{
				BookingID: ap.ID, UserID: actor.ID, Reason: reason,
				Status: model.CancelApproved, Urgent: false,
				ReviewReason: "距服务开始≥2小时，自助取消", CreatedAt: now, ReviewedAt: &now,
			}
			if err := tx.Create(cr).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.Appointment{}).
				Where("id = ? AND status = ?", ap.ID, model.StatusConfirmed).
				UpdateColumn("status", model.StatusCancelled).Error; err != nil {
				return err
			}
			if _, err := b.createRefund(tx, ap.ID, ap.OrderNo, 0, ap.AmountFen, model.RefundKindNormal); err != nil {
				return err
			}
			msg = "自助取消成功，全额退款处理中；退款成功后名额自动释放"
		} else {
			// 人工取消：进入审核，名额持续锁定
			cr = &model.CancelRequest{
				BookingID: ap.ID, UserID: actor.ID, Reason: reason,
				Status: model.CancelPending, Urgent: true, CreatedAt: now,
			}
			if err := tx.Create(cr).Error; err != nil {
				return err
			}
			msg = "距服务开始不足 2 小时，取消申请已提交客服审核；审核期间名额锁定、不可核销"
		}
		repo.WriteAudit(tx, actor.ID, actor.Role, "submit_cancel", ap.ID, cr.Reason)
		return nil
	})
	return cr, msg, err
}

// CancelRequestView 客服审核列表行。
type CancelRequestView struct {
	Request     *model.CancelRequest
	OrderNo     string
	UserPet     string
	SlotRange   string
	StatusText  string
	AmountFen   int64
}

// ListCancelRequests 客服查看取消申请（默认待审核）。
func (b *Biz) ListCancelRequests(ctx context.Context, actor *model.User, status string) ([]CancelRequestView, error) {
	if err := b.requireRole(actor.Role, "support"); err != nil {
		return nil, err
	}
	q := b.DB.Model(&model.CancelRequest{}).
		Select("cancel_requests.*, appointments.order_no, appointments.amount_fen, pets.name AS pet_name, users.username AS user_name").
		Joins("JOIN appointments ON appointments.id = cancel_requests.booking_id").
		Joins("JOIN pets ON pets.id = appointments.pet_id").
		Joins("JOIN users ON users.id = appointments.user_id").
		Order("cancel_requests.id desc")
	if status != "" {
		q = q.Where("cancel_requests.status = ?", status)
	}
	var rows []struct {
		model.CancelRequest
		OrderNo   string
		AmountFen int64
		PetName   string
		UserName  string
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]CancelRequestView, 0, len(rows))
	for i := range rows {
		r := rows[i]
		var ap model.Appointment
		b.DB.First(&ap, r.BookingID)
		var s model.TimeSlot
		b.DB.First(&s, ap.SlotID)
		st := map[string]string{
			model.CancelPending: "待审核", model.CancelApproved: "已通过", model.CancelRejected: "已驳回",
		}[r.Status]
		out = append(out, CancelRequestView{
			Request: &r.CancelRequest, OrderNo: r.OrderNo,
			UserPet: r.UserName + " · " + r.PetName, SlotRange: slotRange(&s),
			StatusText: st, AmountFen: r.AmountFen,
		})
	}
	return out, nil
}

// ReviewCancel 客服审核：批准 → 执行退款；拒绝 → 写明原因、解锁、回到已确认。
func (b *Biz) ReviewCancel(ctx context.Context, actor *model.User, requestID int64, approve bool, reviewReason string) (string, error) {
	if err := b.requireRole(actor.Role, "support"); err != nil {
		return "", err
	}
	if !approve && reviewReason == "" {
		return "", errs.ErrBadRequest.With("驳回必须写明拒绝原因")
	}
	msg := ""
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		var cr model.CancelRequest
		if err := tx.Clauses(locking()).Where("id = ?", requestID).First(&cr).Error; err != nil {
			return errs.ErrCancelRequestNotFound
		}
		if cr.Status != model.CancelPending {
			return errs.ErrCancelAlreadyReviewed
		}
		ap, err := b.getBooking(tx, cr.BookingID)
		if err != nil {
			return err
		}
		now := time.Now()
		if approve {
			// 审核通过：预约置已取消，发起退款（名额在退款成功后原路释放一次）
			res := tx.Model(&model.Appointment{}).
				Where("id = ? AND status = ?", ap.ID, model.StatusConfirmed).
				UpdateColumn("status", model.StatusCancelled)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return errs.ErrStateInvalid.With("预约状态已变化（%s）", statusText(ap.Status))
			}
			tx.Model(&cr).Updates(map[string]interface{}{
				"status": model.CancelApproved, "review_reason": reviewReason, "reviewed_at": now,
			})
			if _, err := b.createRefund(tx, ap.ID, ap.OrderNo, 0, ap.AmountFen, model.RefundKindNormal); err != nil {
				return err
			}
			msg = "已批准取消：退款处理中，退款成功后名额自动释放"
		} else {
			// 审核驳回：名额解除锁定，订单回到已确认，可正常履约核销
			res := tx.Model(&model.CancelRequest{}).
				Where("id = ? AND status = ?", cr.ID, model.CancelPending).
				Updates(map[string]interface{}{
					"status": model.CancelRejected, "review_reason": reviewReason, "reviewed_at": now,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return errs.ErrCancelAlreadyReviewed
			}
			msg = "已驳回取消申请，预约回到已确认状态，可正常核销"
		}
		repo.WriteAudit(tx, actor.ID, actor.Role, "review_cancel", cr.BookingID,
			map[bool]string{true: "批准", false: "驳回"}[approve]+"："+reviewReason)
		return nil
	})
	return msg, err
}
