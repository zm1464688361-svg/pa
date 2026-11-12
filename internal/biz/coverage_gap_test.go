package biz

import (
	"context"
	"testing"
	"time"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
)

// bizErrCode 断言返回的是预期业务错误码。
func bizErrCode(t *testing.T, err error, want *errs.BizError) {
	t.Helper()
	if err == nil {
		t.Fatalf("应返回错误码 %d，实际成功", want.Code)
	}
	be, ok := err.(*errs.BizError)
	if !ok || be.Code != want.Code {
		t.Fatalf("错误码应为 %d，实际 %v", want.Code, err)
	}
}

// TestListSlotsClosedAndStarted 覆盖时段列表的三类不可预约分支与过期时段惰性关闭。
func TestListSlotsClosedAndStarted(t *testing.T) {
	b := newBiz(t)
	now := b.Clock.Now()
	expired := mkSlot(t, b, now.Add(-3*time.Hour), now.Add(-2*time.Hour), 3)
	started := mkSlot(t, b, now.Add(-10*time.Minute), now.Add(50*time.Minute), 3)
	future := mkSlot(t, b, now.Add(26*time.Hour), now.Add(27*time.Hour), 2)

	views, item, err := b.ListSlots(context.Background(), "front")
	if err != nil || item == nil {
		t.Fatalf("ListSlots: %v", err)
	}
	byID := map[int64]SlotView{}
	for _, v := range views {
		byID[v.Slot.ID] = v
	}
	if v := byID[expired.ID]; v.Bookable || v.Reason != "已关闭" {
		t.Fatalf("过期时段应不可预约且已关闭: %+v", v)
	}
	var closed model.TimeSlot
	b.DB.First(&closed, expired.ID)
	if closed.Status != model.SlotClosed {
		t.Fatalf("过期时段应惰性关闭并落库: %s", closed.Status)
	}
	if v := byID[started.ID]; v.Bookable || v.Reason != "时段已开始，不可预约" {
		t.Fatalf("已开始时段应不可预约: %+v", v)
	}
	if v := byID[future.ID]; !v.Bookable {
		t.Fatalf("未来时段应可预约: %+v", v)
	}
	// 客服无权查询时段列表
	if _, _, err := b.ListSlots(context.Background(), "support"); err == nil {
		t.Fatal("客服不应有查询时段权限")
	}
}

// TestCreateBookingSlotGuards 覆盖时段不存在/已过期关闭/已结束三类前置校验。
func TestCreateBookingSlotGuards(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	now := b.Clock.Now()

	// 时段不存在
	_, err := b.CreateBooking(context.Background(), u, 999999, pet.ID)
	bizErrCode(t, err, errs.ErrSlotNotFound)

	// 时段已过期（结束时间已过，即使状态仍为开放）
	ended := mkSlot(t, b, now.Add(-2*time.Hour), now.Add(-time.Hour), 3)
	_, err = b.CreateBooking(context.Background(), u, ended.ID, pet.ID)
	bizErrCode(t, err, errs.ErrSlotExpired)

	// 店长下架（状态关闭）的未来时段
	closed := mkSlot(t, b, now.Add(26*time.Hour), now.Add(27*time.Hour), 3)
	b.DB.Model(closed).UpdateColumn("status", model.SlotClosed)
	_, err = b.CreateBooking(context.Background(), u, closed.ID, pet.ID)
	bizErrCode(t, err, errs.ErrSlotExpired)
}

// TestCreateBookingMissingServiceItem 时段关联的服务项目不存在时应回滚，不扣名额。
func TestCreateBookingMissingServiceItem(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	now := b.Clock.Now()
	slot := mkSlot(t, b, now.Add(26*time.Hour), now.Add(27*time.Hour), 2)
	b.DB.Model(slot).UpdateColumn("service_item_id", 999999)

	if _, err := b.CreateBooking(context.Background(), u, slot.ID, pet.ID); err == nil {
		t.Fatal("服务项目缺失时应失败")
	}
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 2 {
		t.Fatalf("失败后名额应回滚: %d", s.Remaining)
	}
	var cnt int64
	b.DB.Model(&model.Appointment{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("失败不应留下订单: %d", cnt)
	}
}

// TestMarkNoShowGuards 覆盖未到店标记的全部前置校验与成功路径。
func TestMarkNoShowGuards(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	owner := mkUser(t, b, "owner", "owner", 0)
	now := b.Clock.Now()
	slot := mkSlot(t, b, now.Add(26*time.Hour), now.Add(27*time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)

	// 原因必填
	if _, err := b.MarkNoShow(context.Background(), owner, ap.ID, ""); err == nil {
		t.Fatal("空原因应拒绝")
	}
	// 预约不存在
	if _, err := b.MarkNoShow(context.Background(), owner, 999999, "x"); err == nil {
		t.Fatal("不存在的预约应报错")
	}
	// 时段未结束
	bizErrCode(t, mustErr(b.MarkNoShow(context.Background(), owner, ap.ID, "没来")), errs.ErrNoShowNotEnded)
	// 非店长
	front := mkUser(t, b, "front", "front", 1)
	if _, err := b.MarkNoShow(context.Background(), front, ap.ID, "没来"); err == nil {
		t.Fatal("前台不可标记未到店")
	}

	// 时段结束后可标记
	b.Clock.SetOffset(int64(28 * time.Hour / time.Second))
	defer b.Clock.SetOffset(0)
	msg, err := b.MarkNoShow(context.Background(), owner, ap.ID, "客户未到店")
	if err != nil || msg == "" {
		t.Fatalf("标记未到店: %v %q", err, msg)
	}
	var marked model.Appointment
	b.DB.First(&marked, ap.ID)
	if marked.Status != model.StatusNoShow || marked.NoShowReason != "客户未到店" {
		t.Fatalf("标记结果不符: %+v", marked)
	}
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 2 {
		t.Fatalf("未到店不得返还名额: %d", s.Remaining)
	}
	// 重复标记
	bizErrCode(t, mustErr(b.MarkNoShow(context.Background(), owner, ap.ID, "再标一次")), errs.ErrNoShowDone)
}

// TestUpdateNoShowReasonSuccess 未到店原因可编辑，状态不可回退。
func TestUpdateNoShowReasonSuccess(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	owner := mkUser(t, b, "owner", "owner", 0)
	now := b.Clock.Now()
	slot := mkSlot(t, b, now.Add(26*time.Hour), now.Add(27*time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)
	b.Clock.SetOffset(int64(28 * time.Hour / time.Second))
	defer b.Clock.SetOffset(0)
	if _, err := b.MarkNoShow(context.Background(), owner, ap.ID, "首次原因"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if _, err := b.UpdateNoShowReason(context.Background(), owner, ap.ID, ""); err == nil {
		t.Fatal("空原因应拒绝")
	}
	msg, err := b.UpdateNoShowReason(context.Background(), owner, ap.ID, "改为客户临时出差")
	if err != nil || msg == "" {
		t.Fatalf("编辑原因: %v %q", err, msg)
	}
	var got model.Appointment
	b.DB.First(&got, ap.ID)
	if got.NoShowReason != "改为客户临时出差" || got.Status != model.StatusNoShow {
		t.Fatalf("原因应更新且状态不回退: %+v", got)
	}
}

// TestServiceProgressGuards 覆盖履约顺序强制与跨店拦截。
func TestServiceProgressGuards(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	now := b.Clock.Now()
	slot := mkSlot(t, b, now.Add(26*time.Hour), now.Add(27*time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)
	front := mkUser(t, b, "front", "front", 1)

	// 未核销不能登记开始
	bizErrCode(t, mustErr(b.ServiceProgress(context.Background(), front, ap.ID, "start")), errs.ErrStartNotCheckedIn)
	// 未开始不能登记完成
	bizErrCode(t, mustErr(b.ServiceProgress(context.Background(), front, ap.ID, "complete")), errs.ErrCompleteNotStarted)
	// 跨店前台不能核销、不能登记进度
	other := mkUser(t, b, "front2", "front", 2)
	if _, err := b.CheckIn(context.Background(), other, ap.ID); err == nil {
		t.Fatal("跨店核销应拒绝")
	}
	bizErrCode(t, mustErr(b.CheckIn(context.Background(), mkUser(t, b, "front3", "front", 1), 999999)), errs.ErrNotFound)
	b.CheckIn(context.Background(), front, ap.ID)
	bizErrCode(t, mustErr(b.ServiceProgress(context.Background(), other, ap.ID, "start")), errs.ErrForbidden)
}

// TestRefundAbnormalBranches 异常支付退款：权限、不存在、重复处理与成功路径。
func TestRefundAbnormalBranches(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	sup := mkUser(t, b, "support", "support", 0)
	now := b.Clock.Now()
	slot := mkSlot(t, b, now.Add(26*time.Hour), now.Add(27*time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	// 关单后支付成功 → 异常支付待处理
	_, _ = b.CancelUnpaid(context.Background(), u, ap.ID)
	if _, err := payNotify(b, "AB1", ap.OrderNo, "TAB1", 9900, "success"); err != nil {
		t.Fatalf("迟到支付: %v", err)
	}
	var ab model.AbnormalPayment
	if err := b.DB.Where("channel_trade_no = ?", "TAB1").First(&ab).Error; err != nil {
		t.Fatalf("异常支付记录缺失: %v", err)
	}

	// 非客服不可发起
	if _, err := b.RefundAbnormal(context.Background(), u, ab.ID); err == nil {
		t.Fatal("非客服不可发起异常支付退款")
	}
	// 记录不存在
	bizErrCode(t, mustErr(b.RefundAbnormal(context.Background(), sup, 999999)), errs.ErrNotFound)

	msg, err := b.RefundAbnormal(context.Background(), sup, ab.ID)
	if err != nil || msg == "" {
		t.Fatalf("异常支付退款: %v %q", err, msg)
	}
	var after model.AbnormalPayment
	b.DB.First(&after, ab.ID)
	if after.Status != model.AbnormalRefunding {
		t.Fatalf("异常支付应进入退款中: %s", after.Status)
	}
	var refunds int64
	b.DB.Model(&model.Refund{}).Where("kind = ?", model.RefundKindAbnormal).Count(&refunds)
	if refunds == 0 {
		t.Fatal("应生成异常支付退款单")
	}
	// 重复发起
	bizErrCode(t, mustErr(b.RefundAbnormal(context.Background(), sup, ab.ID)), errs.ErrAlreadyDone)

	views, err := b.ListAbnormal(context.Background(), sup)
	if err != nil || len(views) == 0 || views[0].StatusText != "退款中" {
		t.Fatalf("异常支付列表: %v %+v", err, views)
	}
}

// mustErr 丢弃成功返回值，便于直接断言错误码。
func mustErr[T any](_ T, err error) error { return err }
