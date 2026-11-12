package biz

import (
	"context"
	"time"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
)

// DailyReportData 店长日清数据。时段归属按【时段开始时间】落在哪个自然日统计。
type DailyReportData struct {
	Date            string
	ReceivedFen     int64 // 已收款：支付成功金额（不含异常支付）
	RefundedFen     int64 // 已退款：退款回调成功金额
	NetFen          int64 // 实收 = 已收款 − 已退款
	RefundFollowFen int64 // 退款待跟进（处理中/失败/超时）
	CompletedFen    int64 // 已完成服务收入（未到店不计入）
	NoShowCount     int
	NoShowFen       int64
	SlotRows        []SlotStat
	Records         []RecordStat
}

// SlotStat 单时段统计。
type SlotStat struct {
	SlotRange    string
	Valid        int // 待支付 + 已确认
	Completed    int
	Cancelled    int
	NoShow       int
	RefundFollow int // 退款处理中/失败/超时
}

// RecordStat 逐单记录。
type RecordStat struct {
	CreatedAt  string
	OrderNo    string
	UserPet    string
	SlotRange  string
	StatusText string
	Detail     string
	AmountFen  int64
}

// DailyReport 店长查询当日各时段与经营结果。
func (b *Biz) DailyReport(ctx context.Context, actor *model.User, date string) (*DailyReportData, error) {
	if err := b.requireRole(actor.Role, "owner"); err != nil {
		return nil, err
	}
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return nil, errs.ErrBadRequest.With("日期格式应为 yyyy-MM-dd")
	}
	next := day.AddDate(0, 0, 1)
	// 当日时段（按开始时间归属）
	var slots []model.TimeSlot
	if err := b.DB.Where("start_time >= ? AND start_time < ?", day, next).
		Order("start_time asc").Find(&slots).Error; err != nil {
		return nil, err
	}
	slotIDs := make([]int64, 0, len(slots))
	slotByID := map[int64]*model.TimeSlot{}
	for i := range slots {
		slotIDs = append(slotIDs, slots[i].ID)
		slotByID[slots[i].ID] = &slots[i]
	}
	rep := &DailyReportData{Date: date}
	if len(slotIDs) == 0 {
		return rep, nil
	}
	// 当日预约（关联其时段）
	var aps []model.Appointment
	if err := b.DB.Where("slot_id IN ?", slotIDs).Order("id asc").Find(&aps).Error; err != nil {
		return nil, err
	}
	apByID := map[int64]*model.Appointment{}
	for i := range aps {
		apByID[aps[i].ID] = &aps[i]
	}
	// 支付（有效）与退款
	var pays []model.PaymentRecord
	b.DB.Where("kind = ? AND result = ? AND order_no IN (SELECT order_no FROM appointments WHERE slot_id IN ?)",
		model.PayKindNormal, model.PayResultSuccess, slotIDs).Find(&pays)
	var refunds []model.Refund
	b.DB.Where("booking_id IN ?", idsOf(aps)).Find(&refunds)

	receivedByOrder := map[string]int64{}
	for _, p := range pays {
		receivedByOrder[p.OrderNo] += p.AmountFen
	}
	refundByBooking := map[int64]int64{}
	refundFollowByBooking := map[int64]int64{}
	now := b.Clock.Now()
	for i := range refunds {
		r := &refunds[i]
		switch r.Status {
		case model.RefundSuccess:
			refundByBooking[r.BookingID] += r.AmountFen
		case model.RefundProcessing, model.RefundFailed, model.RefundTimeout:
			// 7 自然日超时也留在待跟进
			refundFollowByBooking[r.BookingID] += r.AmountFen
		}
		_ = now
	}
	// 组装
	slotStat := map[int64]*SlotStat{}
	for i := range slots {
		s := &slots[i]
		slotStat[s.ID] = &SlotStat{SlotRange: slotRange(s)}
	}
	for i := range aps {
		ap := &aps[i]
		s := slotByID[ap.SlotID]
		row, ok := slotStat[ap.SlotID]
		if !ok {
			continue
		}
		userPet := b.userPetOf(ap)
		rec := RecordStat{
			CreatedAt: fmtFull(ap.CreatedAt), OrderNo: ap.OrderNo, UserPet: userPet,
			SlotRange: slotRange(s), StatusText: statusText(ap.Status), AmountFen: ap.AmountFen,
		}
		switch ap.Status {
		case model.StatusPendingPayment, model.StatusConfirmed:
			row.Valid++
			if ap.Status == model.StatusConfirmed {
				rec.Detail = "有效预约"
			} else {
				rec.Detail = "待支付（占用名额至 " + fmtFull(ap.PayDeadline) + "）"
			}
		case model.StatusCompleted:
			row.Completed++
			rec.Detail = "已完成服务收入"
		case model.StatusCancelled:
			row.Cancelled++
			rec.Detail = "取消退款"
		case model.StatusNoShow:
			row.NoShow++
			rec.Detail = "未到店：" + ap.NoShowReason
		case model.StatusCheckedIn, model.StatusInService:
			rec.Detail = "履约中"
		default:
			rec.Detail = "已关闭"
		}
		if f := refundFollowByBooking[ap.ID]; f > 0 {
			row.RefundFollow++
			rec.Detail += " · 退款待跟进"
		}
		rep.Records = append(rep.Records, rec)
	}
	for i := range slots {
		rep.SlotRows = append(rep.SlotRows, *slotStat[slots[i].ID])
	}
	// 指标口径
	for i := range aps {
		ap := &aps[i]
		in := receivedByOrder[ap.OrderNo]
		switch ap.Status {
		case model.StatusNoShow:
			rep.NoShowCount++
			rep.NoShowFen += in // 已收款、未退款，单独统计
		case model.StatusCompleted:
			rep.CompletedFen += in
		}
		rep.ReceivedFen += in
		rep.RefundedFen += refundByBooking[ap.ID]
		rep.RefundFollowFen += refundFollowByBooking[ap.ID]
	}
	rep.NetFen = rep.ReceivedFen - rep.RefundedFen
	return rep, nil
}

func (b *Biz) userPetOf(ap *model.Appointment) string {
	var u model.User
	var p model.Pet
	b.DB.First(&u, ap.UserID)
	b.DB.First(&p, ap.PetID)
	return u.Username + " · " + p.Name
}

func idsOf(aps []model.Appointment) []int64 {
	out := make([]int64, 0, len(aps))
	for i := range aps {
		out = append(out, aps[i].ID)
	}
	return out
}
