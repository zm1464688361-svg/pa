package biz

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/sign"
)

// ---------- 取消两分支 ----------

func TestSelfCancelFullRefund(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour) // 距开始 >2h
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)

	if _, _, err := b.SubmitCancel(context.Background(), u, ap.ID, ""); err == nil {
		t.Fatal("自助取消原因也不允许为空？题面仅对<2h要求必填，但空原因一律拒绝更安全")
	}
	cr, msg, err := b.SubmitCancel(context.Background(), u, ap.ID, "临时有事")
	if err != nil || cr.Status != model.CancelApproved {
		t.Fatalf("≥2h 应自助取消成功: %v %+v %q", err, cr, msg)
	}
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.Status != model.StatusCancelled {
		t.Fatalf("预约应为已取消: %s", ap2.Status)
	}
	var r model.Refund
	b.DB.Where("booking_id = ?", ap.ID).First(&r)
	if r.Status != model.RefundProcessing || r.AmountFen != 9900 {
		t.Fatalf("应发起全额退款处理中: %+v", r)
	}
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 2 {
		t.Fatalf("退款成功前名额保持锁定: %d", s.Remaining)
	}
	// 退款成功 → 名额原路释放一次
	_, _ = b.RefundNotify(context.Background(), "RF1", r.RefundNo, "success", sign.Refund("RF1", r.RefundNo, "success"))
	b.DB.First(&s, slot.ID)
	if s.Remaining != 3 {
		t.Fatalf("退款成功后应释放名额: %d", s.Remaining)
	}
	var ap3 model.Appointment
	b.DB.First(&ap3, ap.ID)
	if !ap3.SlotReleased {
		t.Fatal("SlotReleased 标记应为 true")
	}
	// 重复退款成功回调 → 名额不二次释放
	_, _ = b.RefundNotify(context.Background(), "RF1-dup", r.RefundNo, "success", sign.Refund("RF1-dup", r.RefundNo, "success"))
	b.DB.First(&s, slot.ID)
	if s.Remaining != 3 {
		t.Fatalf("名额释放必须幂等: %d", s.Remaining)
	}
}

func TestUrgentCancelRequiresReview(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	// 距开始 1.5 小时（<2h）
	start := b.Clock.Now().Add(90 * time.Minute)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)

	// 空原因拒绝提交
	if _, _, err := b.SubmitCancel(context.Background(), u, ap.ID, ""); err == nil ||
		err.(*errs.BizError).Code != errs.ErrCancelReasonRequired.Code {
		t.Fatalf("空原因必须拒绝: %v", err)
	}
	cr, _, err := b.SubmitCancel(context.Background(), u, ap.ID, "宠物不适")
	if err != nil || cr.Status != model.CancelPending || !cr.Urgent {
		t.Fatalf("应进入客服审核: %v %+v", err, cr)
	}
	// 审核期间不可核销
	front := mkUser(t, b, "front", "front", 1)
	if _, err := b.CheckIn(context.Background(), front, ap.ID); err == nil {
		t.Fatal("审核中取消申请必须拦截核销")
	}
	// 重复提交幂等返回原申请
	cr2, _, _ := b.SubmitCancel(context.Background(), u, ap.ID, "再提交一次")
	if cr2.ID != cr.ID {
		t.Fatalf("重复提交应返回同一申请: %d vs %d", cr.ID, cr2.ID)
	}
	// 审核拒绝 → 回到已确认、可核销；名额锁定解除
	sup := mkUser(t, b, "support", "support", 0)
	msg, err := b.ReviewCancel(context.Background(), sup, cr.ID, false, "当日满档")
	if err != nil {
		t.Fatalf("驳回: %v %q", err, msg)
	}
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.Status != model.StatusConfirmed {
		t.Fatalf("驳回后应回到已确认: %s", ap2.Status)
	}
	if _, err := b.CheckIn(context.Background(), front, ap.ID); err != nil {
		t.Fatalf("驳回后应可核销: %v", err)
	}
	// 重复审核拒绝
	if _, err := b.ReviewCancel(context.Background(), sup, cr.ID, true, ""); err == nil {
		t.Fatal("已审核申请不能重复审核")
	}
}

func TestUrgentCancelApprovedRefunds(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(60 * time.Minute)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)

	cr, _, _ := b.SubmitCancel(context.Background(), u, ap.ID, "临时出差")
	sup := mkUser(t, b, "support", "support", 0)
	// 驳回必须写明原因
	if _, err := b.ReviewCancel(context.Background(), sup, cr.ID, false, ""); err == nil {
		t.Fatal("驳回必须写明原因")
	}
	if _, err := b.ReviewCancel(context.Background(), sup, cr.ID, true, "情况属实"); err != nil {
		t.Fatalf("批准: %v", err)
	}
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.Status != model.StatusCancelled {
		t.Fatalf("批准后应为已取消: %s", ap2.Status)
	}
	var r model.Refund
	b.DB.Where("booking_id = ?", ap.ID).First(&r)
	if r.Status != model.RefundProcessing {
		t.Fatal("批准后应发起退款")
	}
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 2 {
		t.Fatalf("退款成功前名额锁定: %d", s.Remaining)
	}
}

// ---------- 核销履约 ----------

func TestCheckInAndProgress(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)

	frontShop2 := mkUser(t, b, "front2", "front", 2)
	if _, err := b.CheckIn(context.Background(), frontShop2, ap.ID); err == nil {
		t.Fatal("跨店核销必须拒绝")
	}
	front := mkUser(t, b, "front", "front", 1)
	// 禁止跳步
	if _, err := b.ServiceProgress(context.Background(), front, ap.ID, "complete"); err == nil {
		t.Fatal("未核销不能登记完成")
	}
	if _, err := b.ServiceProgress(context.Background(), front, ap.ID, "start"); err == nil {
		t.Fatal("未核销不能登记开始")
	}
	msg, err := b.CheckIn(context.Background(), front, ap.ID)
	if err != nil || msg == "" {
		t.Fatalf("核销: %v %q", err, msg)
	}
	// 重复核销幂等 + 权限
	sup := mkUser(t, b, "support", "support", 0)
	_, err = b.CheckIn(context.Background(), sup, ap.ID)
	if err == nil {
		t.Fatal("客服不能核销（权限矩阵）")
	}
	var alCnt int64
	b.DB.Model(&model.AuditLog{}).Where("action = ? AND target_id = ?", "check_in", ap.ID).Count(&alCnt)
	if alCnt != 1 {
		t.Fatalf("重复核销不得产生新流水: %d", alCnt)
	}
	if _, err := b.ServiceProgress(context.Background(), front, ap.ID, "complete"); err == nil {
		t.Fatal("禁止跳过服务开始直接完成")
	}
	if _, err := b.ServiceProgress(context.Background(), front, ap.ID, "start"); err != nil {
		t.Fatalf("登记开始: %v", err)
	}
	if _, err := b.ServiceProgress(context.Background(), front, ap.ID, "start"); err != nil {
		t.Fatalf("重复开始应幂等: %v", err)
	}
	// 跨天登记完成：拨快时间
	b.Clock.SetOffset(26 * 3600)
	msg, err = b.ServiceProgress(context.Background(), front, ap.ID, "complete")
	if err != nil || msg == "" {
		t.Fatalf("跨天登记完成应允许: %v %q", err, msg)
	}
	b.Clock.SetOffset(0)
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.Status != model.StatusCompleted {
		t.Fatalf("应已完成: %s", ap2.Status)
	}
	// 已完成不能取消
	if _, _, err := b.SubmitCancel(context.Background(), u, ap.ID, "试试"); err == nil {
		t.Fatal("已完成预约不能取消退款")
	}
	if _, err := b.CheckIn(context.Background(), front, ap.ID); err == nil {
		t.Fatal("已完成预约不能再次核销")
	}
}

// TestCheckInVsCancelConcurrent 核销与取消申请并发：先落库者定结果。
func TestCheckInVsCancelConcurrent(t *testing.T) {
	for round := 0; round < 5; round++ {
		b := newBiz(t)
		u := mkUser(t, b, "user1", "pet_owner", 0)
		pet := mkPet(t, b, u.ID, "豆包")
		start := b.Clock.Now().Add(90 * time.Minute)
		slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
		ap := book(t, b, u, slot.ID, pet.ID)
		payOK(t, b, ap)
		front := mkUser(t, b, "front", "front", 1)

		var wg sync.WaitGroup
		var checkInOK, cancelOK bool
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := b.CheckIn(context.Background(), front, ap.ID)
			checkInOK = err == nil
		}()
		go func() {
			defer wg.Done()
			_, _, err := b.SubmitCancel(context.Background(), u, ap.ID, "并发取消")
			cancelOK = err == nil
		}()
		wg.Wait()

		var ap2 model.Appointment
		b.DB.First(&ap2, ap.ID)
		if checkInOK && !cancelOK {
			if ap2.Status != model.StatusCheckedIn {
				t.Fatalf("核销先落库应为已核销: %s", ap2.Status)
			}
		} else if !checkInOK && cancelOK {
			if ap2.Status != model.StatusConfirmed {
				t.Fatalf("取消申请先入库应保持已确认(锁定): %s", ap2.Status)
			}
		} else {
			t.Fatalf("必须恰好一方成功: checkIn=%v cancel=%v status=%s", checkInOK, cancelOK, ap2.Status)
		}
	}
}

// ---------- 未到店 ----------

func TestMarkNoShow(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	// 时段进行中（未结束）
	start := b.Clock.Now().Add(30 * time.Minute)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)
	owner := mkUser(t, b, "owner", "owner", 0)

	if _, err := b.MarkNoShow(context.Background(), owner, ap.ID, "主人未到店"); err == nil ||
		err.(*errs.BizError).Code != errs.ErrNoShowNotEnded.Code {
		t.Fatalf("时段未结束不能标记未到店: %v", err)
	}
	// 等时段结束（拨快 2 小时）
	b.Clock.SetOffset(2 * 3600)
	if _, err := b.MarkNoShow(context.Background(), owner, ap.ID, ""); err == nil {
		t.Fatal("原因必填")
	}
	if _, err := b.MarkNoShow(context.Background(), u, ap.ID, "主人未到店"); err == nil {
		t.Fatal("只有店长可标记")
	}
	msg, err := b.MarkNoShow(context.Background(), owner, ap.ID, "主人未到店且未接电话")
	if err != nil || msg == "" {
		t.Fatalf("店长标记: %v %q", err, msg)
	}
	// 不可回退
	if _, err := b.MarkNoShow(context.Background(), owner, ap.ID, "再标记"); err == nil {
		t.Fatal("标记状态不可回退（重复标记拒绝）")
	}
	// 原因可编辑
	if _, err := b.UpdateNoShowReason(context.Background(), owner, ap.ID, "更新后的原因"); err != nil {
		t.Fatalf("原因可编辑: %v", err)
	}
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.NoShowReason != "更新后的原因" || ap2.Status != model.StatusNoShow {
		t.Fatalf("原因应已更新且状态不变: %+v", ap2)
	}
	// 名额不返还
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 2 {
		t.Fatalf("未到店不返还名额: %d", s.Remaining)
	}
	b.Clock.SetOffset(0)
}

// ---------- 退款重试与超时 ----------

func TestRefundRetryAndFinalFail(t *testing.T) {
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

	// 三次失败重试
	for i := 0; i < 3; i++ {
		if _, err := b.RefundNotify(context.Background(), "F"+r.RefundNo+string(rune('a'+i)), r.RefundNo, "fail", sign.Refund("F"+r.RefundNo+string(rune('a'+i)), r.RefundNo, "fail")); err != nil {
			t.Fatalf("失败回调: %v", err)
		}
		var cur model.Refund
		b.DB.Where("id = ?", r.ID).First(&cur)
		if cur.RetryCount != int32(i+1) {
			t.Fatalf("重试计数: %d", cur.RetryCount)
		}
		if i < 2 {
			if _, err := b.RetryRefund(context.Background(), sup, r.ID); err != nil {
				t.Fatalf("重试: %v", err)
			}
		} else {
			// 第三次失败时仍为 failed；再点重试 → 计数满 3，标终态
			msg, err := b.RetryRefund(context.Background(), sup, r.ID)
			if err != nil {
				t.Fatalf("第四次重试请求: %v %q", err, msg)
			}
			var cur model.Refund
			b.DB.Where("id = ?", r.ID).First(&cur)
			if cur.Status != model.RefundFinalFail {
				t.Fatalf("重试耗尽应标终态失败: %s", cur.Status)
			}
		}
	}
	// 终态后再重试拒绝
	if _, err := b.RetryRefund(context.Background(), sup, r.ID); err == nil {
		t.Fatal("终态失败不再自动/手动重试")
	}
	// 终态失败回调不改变状态
	_, _ = b.RefundNotify(context.Background(), "FINAL", r.RefundNo, "success", sign.Refund("FINAL", r.RefundNo, "success"))
	var cur model.Refund
	b.DB.Where("id = ?", r.ID).First(&cur)
	if cur.Status != model.RefundFinalFail {
		t.Fatalf("终态失败不得被回调复活: %s", cur.Status)
	}
	// 退款失败不影响预约已取消状态，名额未释放
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.Status != model.StatusCancelled || ap2.SlotReleased {
		t.Fatalf("退款失败：预约仍已取消、名额未释放: %+v", ap2)
	}
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 2 {
		t.Fatalf("退款失败不返还名额: %d", s.Remaining)
	}
}

func TestRefundCallbackTimeoutFlag(t *testing.T) {
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
	// 拨快 8 天：超过 7 自然日回调超时
	b.Clock.SetOffset(8 * 24 * 3600)
	views, err := b.ListRefunds(context.Background(), sup, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range views {
		if v.Refund.ID == r.ID {
			found = true
			if !v.TimeoutFlag {
				t.Fatal("超过 7 自然日应标记回调超时")
			}
		}
	}
	if !found {
		t.Fatal("退款单应在客服列表中")
	}
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 2 {
		t.Fatalf("回调超时名额不释放: %d", s.Remaining)
	}
	b.Clock.SetOffset(0)
}

// ---------- 异常支付处理 ----------

func TestAbnormalRefundFlow(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	_, _ = b.CancelUnpaid(context.Background(), u, ap.ID)
	_, _ = payNotify(b, "L1", ap.OrderNo, "TL1", 9900, "success")

	sup := mkUser(t, b, "support", "support", 0)
	items, _ := b.ListAbnormal(context.Background(), sup)
	if len(items) != 1 {
		t.Fatalf("异常支付台账应有 1 条: %d", len(items))
	}
	ab := items[0].Item
	msg, err := b.RefundAbnormal(context.Background(), sup, ab.ID)
	if err != nil || msg == "" {
		t.Fatalf("发起异常退款: %v %q", err, msg)
	}
	// 重复处理拒绝
	if _, err := b.RefundAbnormal(context.Background(), sup, ab.ID); err == nil {
		t.Fatal("已处理异常不得重复退款")
	}
	var r model.Refund
	b.DB.Where("abnormal_id = ?", ab.ID).First(&r)
	if r.Kind != model.RefundKindAbnormal {
		t.Fatalf("应为异常退款单: %+v", r)
	}
	_, _ = b.RefundNotify(context.Background(), "AB1", r.RefundNo, "success", sign.Refund("AB1", r.RefundNo, "success"))
	// 台账永久保存且状态已退款
	items, _ = b.ListAbnormal(context.Background(), sup)
	if len(items) != 1 || items[0].Item.Status != model.AbnormalRefunded {
		t.Fatalf("台账永久保存: %+v", items[0].Item)
	}
	// 不释放名额、不恢复预约
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 3 {
		t.Fatalf("异常退款不返还名额: %d", s.Remaining)
	}
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.Status != model.StatusClosed {
		t.Fatalf("异常退款不恢复预约: %s", ap2.Status)
	}
}

// ---------- 日清报表 ----------

func TestDailyReport(t *testing.T) {
	b := newBiz(t)
	u1 := mkUser(t, b, "user1", "pet_owner", 0)
	u2 := mkUser(t, b, "user2", "pet_owner", 0)
	p1 := mkPet(t, b, u1.ID, "豆包")
	p2 := mkPet(t, b, u2.ID, "毛毛")
	front := mkUser(t, b, "front", "front", 1)
	sup := mkUser(t, b, "support", "support", 0)
	owner := mkUser(t, b, "owner", "owner", 0)
	// 固定基准到次日 10:00：运行时刻靠近午夜时 +3h 不再跨天，用例全时段可重复（缺陷-12）
	base := b.Clock.Now().Add(26 * time.Hour)
	start := time.Date(base.Year(), base.Month(), base.Day(), 10, 0, 0, 0, base.Location())
	day := start.Format("2006-01-02")
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)

	a1 := book(t, b, u1, slot.ID, p1.ID)
	payOK(t, b, a1)
	a2 := book(t, b, u2, slot.ID, p2.ID)
	payOK(t, b, a2)

	// a1 正常履约完成
	b.CheckIn(context.Background(), front, a1.ID)
	b.ServiceProgress(context.Background(), front, a1.ID, "start")
	b.ServiceProgress(context.Background(), front, a1.ID, "complete")
	// a2 临近取消 → 审核批准 → 退款成功
	cr, _, _ := b.SubmitCancel(context.Background(), u2, a2.ID, "出差")
	b.ReviewCancel(context.Background(), sup, cr.ID, true, "同意")
	var r model.Refund
	b.DB.Where("booking_id = ?", a2.ID).First(&r)
	_, _ = b.RefundNotify(context.Background(), "REP1", r.RefundNo, "success", sign.Refund("REP1", r.RefundNo, "success"))

	rep, err := b.DailyReport(context.Background(), owner, day)
	if err != nil {
		t.Fatalf("报表: %v", err)
	}
	if rep.ReceivedFen != 19800 {
		t.Fatalf("已收款应为 19800 分: %d", rep.ReceivedFen)
	}
	if rep.RefundedFen != 9900 {
		t.Fatalf("已退款应为 9900 分: %d", rep.RefundedFen)
	}
	if rep.NetFen != 9900 {
		t.Fatalf("实收应为 9900 分: %d", rep.NetFen)
	}
	if rep.CompletedFen != 9900 {
		t.Fatalf("已完成服务收入应为 9900 分: %d", rep.CompletedFen)
	}
	if len(rep.SlotRows) != 1 {
		t.Fatalf("当日应有 1 个时段: %d", len(rep.SlotRows))
	}
	row := rep.SlotRows[0]
	if row.Completed != 1 || row.Cancelled != 1 || row.Valid != 0 {
		t.Fatalf("时段统计错误: %+v", row)
	}
	// 权限：主人/客服不能看报表
	if _, err := b.DailyReport(context.Background(), u1, day); err == nil {
		t.Fatal("仅店长可查看报表")
	}
	// 未到店口径：再建一单，标记未到店
	a3 := book(t, b, u2, slot.ID, p2.ID)
	if a3 != nil {
		// 同宠物重叠时段，改用另一宠物/另一用户
		u3 := mkUser(t, b, "user3", "pet_owner", 0)
		p3 := mkPet(t, b, u3.ID, "年糕")
		s3 := mkSlot(t, b, start.Add(2*time.Hour), start.Add(3*time.Hour), 3)
		day3 := s3.StartTime.Format("2006-01-02")
		a4 := book(t, b, u3, s3.ID, p3.ID)
		payOK(t, b, a4)
		b.Clock.SetOffset(int64(time.Until(s3.EndTime)/time.Second) + 60)
		b.MarkNoShow(context.Background(), owner, a4.ID, "未到店")
		b.Clock.SetOffset(0)
		rep2, _ := b.DailyReport(context.Background(), owner, day3)
		if rep2.NoShowCount != 1 || rep2.NoShowFen != 9900 {
			t.Fatalf("未到店应单独统计: %+v", rep2)
		}
		if rep2.CompletedFen != 9900 {
			t.Fatalf("已完成服务收入不含未到店（同日还有 1 单已完成）: %d", rep2.CompletedFen)
		}
		if rep2.NetFen != 19800 {
			t.Fatalf("未到店已收款计入实收（未退款）: %d", rep2.NetFen)
		}
	}
}

func TestListCancelRequestsViewAndShopBookings(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(60 * time.Minute)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	payOK(t, b, ap)
	_, _, _ = b.SubmitCancel(context.Background(), u, ap.ID, "出差")

	sup := mkUser(t, b, "support", "support", 0)
	views, err := b.ListCancelRequests(context.Background(), sup, model.CancelPending)
	if err != nil || len(views) != 1 {
		t.Fatalf("待审核列表: %v %d", err, len(views))
	}
	v := views[0]
	if v.OrderNo != ap.OrderNo || v.AmountFen != 9900 || v.StatusText != "待审核" || !v.Request.Urgent {
		t.Fatalf("视图字段: %+v", v)
	}
	views, _ = b.ListCancelRequests(context.Background(), sup, model.CancelApproved)
	if len(views) != 0 {
		t.Fatalf("按状态过滤: %d", len(views))
	}
	// 权限：主人不能看审核台
	if _, err := b.ListCancelRequests(context.Background(), u, ""); err == nil {
		t.Fatal("仅客服可看取消审核台")
	}
	// 前台工作台列表
	front := mkUser(t, b, "front", "front", 1)
	rows, err := b.ListShopBookings(context.Background(), front)
	if err != nil || len(rows) != 1 {
		t.Fatalf("前台当日列表: %v %d", err, len(rows))
	}
	// 主人无前台列表权限
	if _, err := b.ListShopBookings(context.Background(), u); err == nil {
		t.Fatal("仅前台可看工作台")
	}
}

// payOK 用有效支付通知把订单推到已确认。
func payOK(t *testing.T, b *Biz, ap *model.Appointment) {
	t.Helper()
	notifyID := "OK-" + ap.OrderNo
	tradeNo := "T-" + ap.OrderNo
	if _, err := payNotify(b, notifyID, ap.OrderNo, tradeNo, ap.AmountFen, "success"); err != nil {
		t.Fatalf("支付通知: %v", err)
	}
}
