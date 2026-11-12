package biz

import (
	"context"
	"fmt"
	"time"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/repo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// createRefund 创建退款单（取消退款 / 异常支付退款共用）。
func (b *Biz) createRefund(tx *gorm.DB, bookingID int64, orderNo string, abnormalID, amountFen int64, kind string) (*model.Refund, error) {
	r := model.Refund{
		RefundNo: genRefundNo(), BookingID: bookingID, OrderNo: orderNo,
		AbnormalID: abnormalID, AmountFen: amountFen,
		Status: model.RefundProcessing, Kind: kind, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := tx.Create(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// RefundNotify 退款结果回调。
// 成功：退款单置 success；普通取消退款释放名额（幂等一次）；异常支付退款置已退款。
// 失败：置 failed（可重试）；重复失败通知不重复累计。
func (b *Biz) RefundNotify(ctx context.Context, notifyID, refundNo, result, sig string) (string, error) {
	if err := checkRefundSign(notifyID, refundNo, result, sig); err != nil {
		return "", err
	}
	msg := ""
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		var r model.Refund
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("refund_no = ?", refundNo).First(&r).Error; err != nil {
			return errs.ErrRefundNotFound
		}
		switch {
		case r.Status == model.RefundSuccess:
			msg = "重复回调，退款已是成功终态"
			return nil
		case r.Status == model.RefundFinalFail:
			msg = "退款已终态失败，回调不再改变状态"
			return nil
		}
		if result == model.PayResultSuccess {
			res := tx.Model(&model.Refund{}).
				Where("id = ? AND status IN ?", r.ID,
					[]string{model.RefundProcessing, model.RefundFailed, model.RefundTimeout}).
				Updates(map[string]interface{}{"status": model.RefundSuccess, "fail_reason": "", "updated_at": time.Now()})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 1 {
				msg = "退款成功"
				if r.Kind == model.RefundKindNormal && r.BookingID > 0 {
					// 名额原路返还，仅一次
					var ap model.Appointment
					if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
						First(&ap, r.BookingID).Error; err == nil {
						b.releaseSlotOnce(tx, &ap)
					}
				}
				if r.Kind == model.RefundKindAbnormal && r.AbnormalID > 0 {
					tx.Model(&model.AbnormalPayment{}).Where("id = ?", r.AbnormalID).
						Update("status", model.AbnormalRefunded)
				}
				repo.WriteAudit(tx, 0, "channel", "refund_success", r.BookingID, r.RefundNo+" 退款 "+itoa(r.AmountFen)+" 分")
			} else {
				msg = "重复回调，已按首次结果处理"
			}
			return nil
		}
		// 渠道明确失败
		res := tx.Model(&model.Refund{}).
			Where("id = ? AND status = ?", r.ID, model.RefundProcessing).
			Updates(map[string]interface{}{
				"status": model.RefundFailed, "retry_count": gorm.Expr("retry_count + 1"),
				"fail_reason": "渠道返回失败", "updated_at": time.Now(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			msg = "退款失败，可在客服页重试"
		} else {
			msg = "重复失败回调，已按首次结果处理"
		}
		return nil
	})
	return msg, err
}

// RefundView 退款展示行（含超时判定：处理中超过 7 自然日显示超时）。
type RefundView struct {
	Refund      *model.Refund
	StatusText  string
	TimeoutFlag bool
}

func refundStatusText(st string) string {
	switch st {
	case model.RefundProcessing:
		return "退款处理中"
	case model.RefundSuccess:
		return "退款成功"
	case model.RefundFailed:
		return "退款失败（可重试）"
	case model.RefundFinalFail:
		return "退款终态失败（人工台账）"
	case model.RefundTimeout:
		return "退款回调超时"
	}
	return st
}

// listRefundViews 通用查询并标注超时。
func (b *Biz) listRefundViews(status string) ([]RefundView, error) {
	q := b.DB.Order("id desc")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var refunds []model.Refund
	if err := q.Find(&refunds).Error; err != nil {
		return nil, err
	}
	now := b.Clock.Now()
	out := make([]RefundView, 0, len(refunds))
	for i := range refunds {
		r := &refunds[i]
		v := RefundView{Refund: r, StatusText: refundStatusText(r.Status)}
		// 回调超时阈值 7 自然日：处理中超期显示超时并留待跟进
		if r.Status == model.RefundProcessing && now.Sub(r.CreatedAt) > refundTimeout {
			v.TimeoutFlag = true
			v.StatusText = "退款回调超时（7 自然日），持续待跟进"
		}
		out = append(out, v)
	}
	return out, nil
}

// ListRefunds 客服查询退款单。
func (b *Biz) ListRefunds(ctx context.Context, actor *model.User, status string) ([]RefundView, error) {
	if err := b.requireRole(actor.Role, "support"); err != nil {
		return nil, err
	}
	return b.listRefundViews(status)
}

// RetryRefund 客服重试退款：最多 3 次，耗尽标终态失败。
func (b *Biz) RetryRefund(ctx context.Context, actor *model.User, refundID int64) (string, error) {
	if err := b.requireRole(actor.Role, "support"); err != nil {
		return "", err
	}
	msg := ""
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		var r model.Refund
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, refundID).Error; err != nil {
			return errs.ErrRefundNotFound
		}
		switch r.Status {
		case model.RefundFinalFail:
			return errs.ErrRefundRetried
		case model.RefundSuccess:
			return errs.ErrAlreadyDone.With("退款已成功")
		}
		if r.RetryCount >= maxRefundRetry {
			tx.Model(&model.Refund{}).Where("id = ?", r.ID).
				Updates(map[string]interface{}{"status": model.RefundFinalFail, "updated_at": time.Now()})
			msg = "已达最大重试次数，标记退款终态失败，转人工台账"
			repo.WriteAudit(tx, actor.ID, actor.Role, "refund_final_fail", r.BookingID, r.RefundNo)
			return nil
		}
		tx.Model(&model.Refund{}).Where("id = ?", r.ID).
			Updates(map[string]interface{}{"status": model.RefundProcessing, "updated_at": time.Now()})
		msg = fmt.Sprintf("已发起第 %d 次重试，请在模拟渠道触发退款结果", r.RetryCount+1)
		repo.WriteAudit(tx, actor.ID, actor.Role, "refund_retry", r.BookingID, r.RefundNo)
		return nil
	})
	return msg, err
}
