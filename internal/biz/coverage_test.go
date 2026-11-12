package biz

import (
	"context"
	"testing"
	"time"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/sign"
)

// ---------- 查询与权限 ----------

func TestListBookingsPermission(t *testing.T) {
	b := newBiz(t)
	u1 := mkUser(t, b, "user1", "pet_owner", 0)
	u2 := mkUser(t, b, "user2", "pet_owner", 0)
	p1 := mkPet(t, b, u1.ID, "豆包")
	p2 := mkPet(t, b, u2.ID, "毛毛")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	a1 := book(t, b, u1, slot.ID, p1.ID)
	book(t, b, u2, slot.ID, p2.ID)
	payOK(t, b, a1)

	rows, err := b.ListBookings(context.Background(), u1, 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("主人只能看自己的单: %v %d", err, len(rows))
	}
	owner := mkUser(t, b, "owner", "owner", 0)
	rows, _ = b.ListBookings(context.Background(), owner, slot.ID)
	if len(rows) != 2 {
		t.Fatalf("店长按时段看本店全部: %d", len(rows))
	}
	front := mkUser(t, b, "front", "front", 1)
	rows, _ = b.ListBookings(context.Background(), front, 0)
	if len(rows) != 2 {
		t.Fatalf("前台看本店: %d", len(rows))
	}
	if _, err := b.ListBookings(context.Background(), mkUser(t, b, "support", "support", 0), 0); err == nil {
		t.Fatal("客服无全店预约列表权限")
	}
	// 详情权限
	if _, _, err := b.GetBookingDetail(context.Background(), u2, a1.ID); err == nil {
		t.Fatal("不能看他人订单详情")
	}
	if _, st, err := b.GetBookingDetail(context.Background(), u1, a1.ID); err != nil || st == "" {
		t.Fatalf("本人详情: %v %s", err, st)
	}
	if _, _, err := b.GetBookingDetail(context.Background(), front, a1.ID); err != nil {
		t.Fatalf("前台可看本店详情: %v", err)
	}
}

func TestListRefundsPermission(t *testing.T) {
	b := newBiz(t)
	if _, err := b.ListRefunds(context.Background(), mkUser(t, b, "ownerR", "owner", 0), ""); err == nil {
		t.Fatal("仅客服可查退款")
	}
	if _, err := b.ListAbnormal(context.Background(), mkUser(t, b, "ownerA", "owner", 0)); err == nil {
		t.Fatal("仅客服可查异常支付")
	}
}

// ---------- 支付分支补充 ----------

func TestPayNotifyFailResultRecorded(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	msg, err := payNotify(b, "PF1", ap.OrderNo, "TF1", 9900, "fail")
	if err != nil || msg == "" {
		t.Fatalf("失败通知受理: %v %q", err, msg)
	}
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.Status != model.StatusPendingPayment {
		t.Fatalf("失败通知不改变状态: %s", ap2.Status)
	}
	var rec model.PaymentRecord
	b.DB.Where("notify_id = ?", "PF1").First(&rec)
	if rec.Kind != model.PayKindRejected {
		t.Fatalf("失败通知应归类 rejected: %s", rec.Kind)
	}
	// 支付后进行中状态的重复成功通知 → 异常支付（dup）
	payOK(t, b, ap)
	_, _ = payNotify(b, "PD1", ap.OrderNo, "TD1", 9900, "success")
	var ab model.AbnormalPayment
	b.DB.Where("channel_trade_no = ?", "TD1").First(&ab)
	if ab.Reason != model.PayKindDup {
		t.Fatalf("已确认订单的再次成功支付应入异常: %s", ab.Reason)
	}
}

func TestDuplicateTradeNoAbnormalIdempotent(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	_, _ = b.CancelUnpaid(context.Background(), u, ap.ID)
	// 相同交易号两次迟到支付：台账只登记一次
	_, _ = payNotify(b, "DT1", ap.OrderNo, "TT1", 9900, "success")
	_, err := payNotify(b, "DT2", ap.OrderNo, "TT1", 9900, "success")
	if err != nil {
		t.Fatalf("同交易号重复通知应幂等处理: %v", err)
	}
	var n int64
	b.DB.Model(&model.AbnormalPayment{}).Where("channel_trade_no = ?", "TT1").Count(&n)
	if n != 1 {
		t.Fatalf("同一交易号只登记一次异常: %d", n)
	}
}

// ---------- 审核与履约补充 ----------

func TestReviewCancelStateInvalid(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(60 * time.Minute)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)
	cr, _, _ := b.SubmitCancel(context.Background(), u, ap.ID, "出差")
	// 模拟并发场景：核销先落库（直接置库内状态），审核批准应状态冲突
	b.DB.Model(&model.Appointment{}).Where("id = ?", ap.ID).
		Update("status", model.StatusCheckedIn)
	sup := mkUser(t, b, "support", "support", 0)
	if _, err := b.ReviewCancel(context.Background(), sup, cr.ID, true, "同意"); err == nil {
		t.Fatal("已核销的预约不允许批准取消退款")
	}
}

func TestMarkNoShowStateInvalid(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)
	owner := mkUser(t, b, "owner", "owner", 0)
	// 已完成预约不可标记未到店
	front := mkUser(t, b, "front", "front", 1)
	b.CheckIn(context.Background(), front, ap.ID)
	b.ServiceProgress(context.Background(), front, ap.ID, "start")
	b.ServiceProgress(context.Background(), front, ap.ID, "complete")
	b.Clock.SetOffset(2 * 3600)
	if _, err := b.MarkNoShow(context.Background(), owner, ap.ID, "原因"); err == nil {
		t.Fatal("已完成预约不可标记未到店")
	}
	// 非未到店订单不可编辑原因
	if _, err := b.UpdateNoShowReason(context.Background(), owner, ap.ID, "x"); err == nil {
		t.Fatal("仅未到店可编辑原因")
	}
	b.Clock.SetOffset(0)
}

func TestServiceProgressInvalidAction(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)
	front := mkUser(t, b, "front", "front", 1)
	if _, err := b.ServiceProgress(context.Background(), front, ap.ID, "noop"); err == nil {
		t.Fatal("未知 action 应拒绝")
	}
}

func TestCancelUnpaidWrongState(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)
	if _, err := b.CancelUnpaid(context.Background(), u, ap.ID); err == nil {
		t.Fatal("已支付订单不能用待支付取消通道")
	}
	// 店长不能用主人通道
	if _, err := b.CancelUnpaid(context.Background(), mkUser(t, b, "owner", "owner", 0), ap.ID); err == nil {
		t.Fatal("仅主人可取消待支付单")
	}
	// 主人不能提交他人预约的取消
	u3 := mkUser(t, b, "user3", "pet_owner", 0)
	if _, _, err := b.SubmitCancel(context.Background(), u3, ap.ID, "x"); err == nil {
		t.Fatal("不能提交他人预约取消")
	}
	// 未知预约详情
	if _, _, err := b.GetBookingDetail(context.Background(), u, 99999); err == nil {
		t.Fatal("未知订单应报不存在")
	}
	if _, _, err := b.GetBookingDetail(context.Background(), mkUser(t, b, "user2", "pet_owner", 0), 99999); err == nil {
		t.Fatal("未知订单应报不存在")
	}
}

// ---------- 退款超时分支（非超时正常处理中不标记） ----------

func TestRefundNotTimeoutWithin7Days(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)
	cr, _, _ := b.SubmitCancel(context.Background(), u, ap.ID, "有事")
	sup := mkUser(t, b, "support", "support", 0)
	b.ReviewCancel(context.Background(), sup, cr.ID, true, "同意")
	var r model.Refund
	b.DB.Where("booking_id = ?", ap.ID).First(&r)
	b.Clock.SetOffset(int64(6*24*time.Hour/time.Second) + 3600)
	views, _ := b.ListRefunds(context.Background(), sup, "")
	for _, v := range views {
		if v.Refund.ID == r.ID && v.TimeoutFlag {
			t.Fatal("7 自然日内不应标记超时")
		}
	}
	b.Clock.SetOffset(0)
}

// ---------- 报表空日期与日期解析 ----------

func TestDailyReportEdge(t *testing.T) {
	b := newBiz(t)
	owner := mkUser(t, b, "owner", "owner", 0)
	if _, err := b.DailyReport(context.Background(), owner, "2026-99-99"); err == nil {
		t.Fatal("非法日期应拒绝")
	}
	rep, err := b.DailyReport(context.Background(), owner, time.Now().Format("2006-01-02"))
	if err != nil || rep.ReceivedFen != 0 {
		t.Fatalf("空日期应返回零值报表: %v %+v", err, rep)
	}
	// 验签错误路径：空签名
	if _, err := b.PayNotify(context.Background(), "NS1", "B1", "T1", 9900, "success", ""); err == nil ||
		err.(*errs.BizError).Code != errs.ErrPaySign.Code {
		t.Fatalf("空签名应报验签失败: %v", err)
	}
	if _, err := b.RefundNotify(context.Background(), "NS2", "R1", "success", ""); err == nil {
		t.Fatal("退款空签名应拒绝")
	}
	if _, err := b.PayNotify(context.Background(), "NS3", "B1", "T1", 9900, "success",
		sign.Pay("NS3", "B1", "T1", 9900, "success")); err == nil {
		t.Fatal("未知订单应拒绝（留痕在事务外）")
	}
}

// ---------- 文案函数 ----------

func TestStatusTexts(t *testing.T) {
	cases := map[string]string{
		model.StatusPendingPayment: "待支付", model.StatusClosed: "已关闭",
		model.StatusConfirmed: "已确认", model.StatusCancelled: "已取消",
		model.StatusCheckedIn: "已核销", model.StatusInService: "服务中",
		model.StatusCompleted: "已完成", model.StatusNoShow: "未到店",
		"weird": "weird",
	}
	for st, want := range cases {
		if got := statusText(st); got != want {
			t.Fatalf("statusText(%s)=%s want %s", st, got, want)
		}
	}
	rc := map[string]string{
		model.RefundProcessing: "退款处理中", model.RefundSuccess: "退款成功",
		model.RefundFailed: "退款失败（可重试）", model.RefundFinalFail: "退款终态失败（人工台账）",
		model.RefundTimeout: "退款回调超时",
	}
	for st, want := range rc {
		if got := refundStatusText(st); got != want {
			t.Fatalf("refundStatusText(%s)=%s", st, got)
		}
	}
	ab := map[string]string{
		model.AbnormalPending: "待核实退款", model.AbnormalRefunding: "退款中", model.AbnormalRefunded: "已退款",
	}
	for st, want := range ab {
		if got := abnormalText(st); got != want {
			t.Fatalf("abnormalText(%s)=%s", st, got)
		}
	}
}

// sign 导入占用检查
var _ = sign.Pay
