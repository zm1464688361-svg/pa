package biz

import (
	"context"
	"time"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/repo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PayNotify 支付结果通知（pay_sim 异步回调，经网关转发）。
// 规则：验签 → 幂等（notifyId 唯一）→ 匹配订单与金额 → 有效支付确认预约；
// 重复通知不重复确认；关单后支付进入异常支付台账。
func (b *Biz) PayNotify(ctx context.Context, notifyID, orderNo, tradeNo string, amountFen int64, result, sig string) (string, error) {
	// 1) 验签
	if err := checkPaySign(notifyID, orderNo, tradeNo, amountFen, result, sig); err != nil {
		return "", err
	}
	msg := ""
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		// 2) 幂等键：notify_id 唯一，重复通知返回首次结果
		var existed model.PaymentRecord
		if err := tx.Where("notify_id = ?", notifyID).First(&existed).Error; err == nil {
			msg = "重复通知，已按首次结果处理：" + existed.Kind
			return nil
		}
		// 3) 锁订单并惰性关单
		var ap model.Appointment
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_no = ?", orderNo).First(&ap).Error
		if errorsIs(err) {
			return errs.ErrPayUnknown
		}
		if err != nil {
			return err
		}
		b.expireIfDue(tx, &ap)
		// 4) 渠道明确失败：仅记流水
		if result != model.PayResultSuccess {
			rec := model.PaymentRecord{NotifyID: notifyID, OrderNo: orderNo, ChannelTradeNo: tradeNo,
				AmountFen: amountFen, Result: result, Kind: model.PayKindRejected, Detail: "渠道返回失败", CreatedAt: time.Now()}
			tx.Create(&rec)
			msg = "支付失败通知已登记"
			return nil
		}
		// 5) 金额校验
		if amountFen != ap.AmountFen {
			return errs.ErrPayAmount.With("应 %d 实 %d", ap.AmountFen, amountFen)
		}
		switch ap.Status {
		case model.StatusPendingPayment:
			// 6) 有效支付：确认预约（状态条件更新保证只确认一次）
			res := tx.Model(&model.Appointment{}).
				Where("id = ? AND status = ?", ap.ID, model.StatusPendingPayment).
				Updates(map[string]interface{}{"status": model.StatusConfirmed, "updated_at": time.Now()})
			if res.Error != nil {
				return res.Error
			}
			rec := model.PaymentRecord{NotifyID: notifyID, OrderNo: orderNo, ChannelTradeNo: tradeNo,
				AmountFen: amountFen, Result: result, Kind: model.PayKindNormal, CreatedAt: time.Now()}
			if err := tx.Create(&rec).Error; err != nil {
				return err
			}
			if res.RowsAffected == 1 {
				msg = "支付成功，预约已确认"
				repo.WriteAudit(tx, 0, "channel", "pay_success", ap.ID, orderNo+" 支付 "+itoa(amountFen)+" 分")
			} else {
				// 并发下另一笔通知已完成确认：本笔划入重复支付
				if err := b.toAbnormal(tx, orderNo, tradeNo, amountFen, model.PayKindDup); err != nil {
					return err
				}
				msg = "重复支付，已划入异常支付待处理"
			}
		case model.StatusClosed:
			// 7) 关单后迟到支付：异常支付台账，不恢复预约
			if err := b.toAbnormal(tx, orderNo, tradeNo, amountFen, "closed_order_paid"); err != nil {
				return err
			}
			msg = "订单已关闭，支付款项登记异常支付待退款"
		case model.StatusConfirmed, model.StatusCheckedIn, model.StatusInService, model.StatusCompleted:
			if err := b.toAbnormal(tx, orderNo, tradeNo, amountFen, model.PayKindDup); err != nil {
				return err
			}
			msg = "重复支付，已划入异常支付待处理"
		default:
			rec := model.PaymentRecord{NotifyID: notifyID, OrderNo: orderNo, ChannelTradeNo: tradeNo,
				AmountFen: amountFen, Result: result, Kind: model.PayKindRejected,
				Detail: "状态 " + ap.Status + " 不受理支付", CreatedAt: time.Now()}
			tx.Create(&rec)
			return errs.ErrStateInvalid
		}
		return nil
	})
	// 验签通过但被拒绝的通知（未知订单/金额不符）：在事务外补记流水，保证可查询处理结果
	if err != nil && (err == errs.ErrPayUnknown || isBiz(err, errs.ErrPayAmount.Code)) {
		rec := model.PaymentRecord{NotifyID: notifyID, OrderNo: orderNo, ChannelTradeNo: tradeNo,
			AmountFen: amountFen, Result: result, Kind: model.PayKindRejected,
			Detail: "已拒绝：" + err.Error(), CreatedAt: time.Now()}
		b.DB.Create(&rec)
	}
	return msg, err
}

// isBiz 判断是否指定 Code 的业务错误。
func isBiz(err error, code int) bool {
	be, ok := err.(*errs.BizError)
	return ok && be.Code == code
}

// toAbnormal 登记异常支付台账。channelTradeNo 唯一；冲突时降级记流水。
func (b *Biz) toAbnormal(tx *gorm.DB, orderNo, tradeNo string, amountFen int64, reason string) error {
	rec := model.PaymentRecord{NotifyID: "dup-" + tradeNo, OrderNo: orderNo, ChannelTradeNo: tradeNo,
		AmountFen: amountFen, Result: model.PayResultSuccess, Kind: reason, CreatedAt: time.Now()}
	if err := tx.Create(&rec).Error; err != nil {
		// 同一交易号重复登记（幂等护栏）
		return nil
	}
	ap := model.AbnormalPayment{
		OrderNo: orderNo, ChannelTradeNo: tradeNo, AmountFen: amountFen,
		Reason: reason, Status: model.AbnormalPending, CreatedAt: time.Now(),
	}
	return tx.Create(&ap).Error
}

// errorsIs 判断是否记录不存在。
func errorsIs(err error) bool { return err == gorm.ErrRecordNotFound }
