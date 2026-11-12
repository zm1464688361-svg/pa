package biz

import (
	"context"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/repo"
	"gorm.io/gorm"
)

// AbnormalView 异常支付展示行。
type AbnormalView struct {
	Item       *model.AbnormalPayment
	StatusText string
}

func abnormalText(st string) string {
	switch st {
	case model.AbnormalPending:
		return "待核实退款"
	case model.AbnormalRefunding:
		return "退款中"
	case model.AbnormalRefunded:
		return "已退款"
	}
	return st
}

// ListAbnormal 客服查看异常支付台账（永久保存，不随退款成功删除）。
func (b *Biz) ListAbnormal(ctx context.Context, actor *model.User) ([]AbnormalView, error) {
	if err := b.requireRole(actor.Role, "support"); err != nil {
		return nil, err
	}
	var items []model.AbnormalPayment
	if err := b.DB.Order("id desc").Find(&items).Error; err != nil {
		return nil, err
	}
	out := make([]AbnormalView, 0, len(items))
	for i := range items {
		out = append(out, AbnormalView{Item: &items[i], StatusText: abnormalText(items[i].Status)})
	}
	return out, nil
}

// RefundAbnormal 客服核实异常支付后发起退款（退款重试规则与普通退款一致）。
func (b *Biz) RefundAbnormal(ctx context.Context, actor *model.User, abnormalID int64) (string, error) {
	if err := b.requireRole(actor.Role, "support"); err != nil {
		return "", err
	}
	msg := ""
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		var ab model.AbnormalPayment
		if err := tx.Clauses(locking()).First(&ab, abnormalID).Error; err != nil {
			return errs.ErrNotFound.With("异常支付记录 %d", abnormalID)
		}
		if ab.Status != model.AbnormalPending {
			return errs.ErrAlreadyDone.With("该异常支付已处理（%s）", abnormalText(ab.Status))
		}
		// 客服核实动作：核对商户订单号与交易号（页面展示供核对，此处记录审计）
		if _, err := b.createRefund(tx, 0, ab.OrderNo, ab.ID, ab.AmountFen, model.RefundKindAbnormal); err != nil {
			return err
		}
		if err := tx.Model(&ab).Update("status", model.AbnormalRefunding).Error; err != nil {
			return err
		}
		msg = "已发起异常支付退款（原路退回 " + itoa(ab.AmountFen) + " 分）；不释放名额、不恢复预约"
		repo.WriteAudit(tx, actor.ID, actor.Role, "refund_abnormal", ab.ID, ab.OrderNo+" / "+ab.ChannelTradeNo)
		return nil
	})
	return msg, err
}

var _ = gorm.ErrRecordNotFound
