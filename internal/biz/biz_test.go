package biz

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/zm1464688361-svg/pa/internal/config"
	"github.com/zm1464688361-svg/pa/internal/clock"
	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/repo"
	"github.com/zm1464688361-svg/pa/internal/sign"
)

// newBiz 建立独立测试库（petshop_test）并清空业务表。
func newBiz(t *testing.T) *Biz {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = config.LoadEnvKey("test_dsn")
	}
	if dsn == "" {
		t.Skip("未配置测试数据库：设置环境变量 TEST_DB_DSN 或 config.local.yaml 的 test_dsn")
	}
	db, err := repo.Open(dsn, true)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	for _, tbl := range []string{
		"audit_logs", "payment_records", "refunds", "cancel_requests",
		"abnormal_payments", "appointments", "time_slots", "service_items",
		"pets", "users", "shops", "clock_offsets",
	} {
		if err := db.Exec("SET FOREIGN_KEY_CHECKS=0").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s", tbl)).Error; err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
	db.Exec("INSERT INTO shops (id,name) VALUES (1,'测试门店')")
	db.Create(&model.ServiceItem{ShopID: 1, Name: "基础洗护", PriceFen: 9900})
	b := &Biz{DB: db, Clock: clock.New(nil)}
	t.Cleanup(func() { _ = db })
	return b
}

func mkUser(t *testing.T, b *Biz, name, role string, shopID int64) *model.User {
	t.Helper()
	u := model.User{Username: name, Role: role, ShopID: shopID}
	if err := b.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return &u
}

func mkPet(t *testing.T, b *Biz, userID int64, name string) *model.Pet {
	t.Helper()
	p := model.Pet{UserID: userID, Name: name, Breed: "柯基"}
	if err := b.DB.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	return &p
}

func mkSlot(t *testing.T, b *Biz, start, end time.Time, cap int32) *model.TimeSlot {
	t.Helper()
	s := model.TimeSlot{ShopID: 1, ServiceItemID: 1, StartTime: start, EndTime: end,
		Capacity: cap, Remaining: cap, Status: model.SlotOpen}
	if err := b.DB.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	return &s
}

func book(t *testing.T, b *Biz, u *model.User, slotID, petID int64) *model.Appointment {
	t.Helper()
	ap, err := b.CreateBooking(context.Background(), u, slotID, petID)
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}
	return ap
}

// ---------- 时段 ----------

func TestCreateSlotValidation(t *testing.T) {
	b := newBiz(t)
	owner := mkUser(t, b, "owner", "owner", 0)
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	if _, err := b.CreateSlot(context.Background(), owner.ID, "pet_owner", tomorrow+" 10:00", tomorrow+" 11:00", 3); err == nil {
		t.Fatal("非店长不能发布时段")
	}
	if _, err := b.CreateSlot(context.Background(), owner.ID, "owner", tomorrow+" 11:00", tomorrow+" 10:00", 3); err == nil {
		t.Fatal("结束早于开始应拒绝")
	}
	if _, err := b.CreateSlot(context.Background(), owner.ID, "owner", tomorrow+" 10:00", tomorrow+" 11:00", 0); err == nil {
		t.Fatal("名额必须大于 0")
	}
	s, err := b.CreateSlot(context.Background(), owner.ID, "owner", tomorrow+" 10:00", tomorrow+" 11:00", 3)
	if err != nil || s.Capacity != 3 || s.Remaining != 3 {
		t.Fatalf("合法发布失败: %v", err)
	}
}

func TestListSlotsBookable(t *testing.T) {
	b := newBiz(t)
	owner := mkUser(t, b, "owner", "owner", 0)
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	full, _ := b.CreateSlot(context.Background(), owner.ID, "owner", tomorrow+" 10:00", tomorrow+" 11:00", 2)
	b.DB.Model(full).UpdateColumn("remaining", 0)
	okSlot, _ := b.CreateSlot(context.Background(), owner.ID, "owner", tomorrow+" 14:00", tomorrow+" 15:00", 3)

	views, item, err := b.ListSlots(context.Background(), "pet_owner")
	if err != nil || item.PriceFen != 9900 || len(views) != 2 {
		t.Fatalf("ListSlots: %v len=%d", err, len(views))
	}
	byID := map[int64]SlotView{}
	for _, v := range views {
		byID[v.Slot.ID] = v
	}
	if byID[full.ID].Bookable || byID[full.ID].Reason != "名额已满" {
		t.Fatalf("满员时段应不可预约: %+v", byID[full.ID])
	}
	if !byID[okSlot.ID].Bookable {
		t.Fatalf("正常时段应可预约")
	}
}

// ---------- 预约资格 ----------

func TestCreateBookingSuccessAndHold(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)

	ap := book(t, b, u, slot.ID, pet.ID)
	if ap.Status != model.StatusPendingPayment {
		t.Fatalf("应为待支付: %s", ap.Status)
	}
	if ap.AmountFen != 9900 {
		t.Fatalf("金额应为 9900 分: %d", ap.AmountFen)
	}
	// 计时起点 = 订单创建时间，占用 5 分钟
	if !ap.PayDeadline.After(b.Clock.Now().Add(4*time.Minute)) {
		t.Fatalf("PayDeadline 应为创建时间 +5 分钟: %v", ap.PayDeadline)
	}
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 2 {
		t.Fatalf("提交预约应预扣名额: remaining=%d", s.Remaining)
	}
}

func TestCreateBookingRejects(t *testing.T) {
	b := newBiz(t)
	u1 := mkUser(t, b, "user1", "pet_owner", 0)
	u2 := mkUser(t, b, "user2", "pet_owner", 0)
	p1 := mkPet(t, b, u1.ID, "豆包")
	p2 := mkPet(t, b, u2.ID, "毛毛")
	start := b.Clock.Now().Add(26 * time.Hour)
	full := mkSlot(t, b, start, start.Add(time.Hour), 1)
	b.DB.Model(full).UpdateColumn("remaining", 0)
	started := mkSlot(t, b, b.Clock.Now().Add(-time.Minute), b.Clock.Now().Add(59*time.Minute), 3)
	other := mkSlot(t, b, start.Add(30*time.Minute), start.Add(90*time.Minute), 3)

	if _, err := b.CreateBooking(context.Background(), u1, full.ID, p1.ID); err == nil || err.(*errs.BizError).Code != errs.ErrSlotFull.Code {
		t.Fatalf("名额已满应拒绝: %v", err)
	}
	if _, err := b.CreateBooking(context.Background(), u1, started.ID, p1.ID); err == nil || err.(*errs.BizError).Code != errs.ErrSlotStarted.Code {
		t.Fatalf("时段已开始应拒绝: %v", err)
	}
	if _, err := b.CreateBooking(context.Background(), u1, other.ID, 9999); err == nil || err.(*errs.BizError).Code != errs.ErrPetNotYours.Code {
		t.Fatalf("非本人宠物应拒绝: %v", err)
	}
	// 同宠物同时段重复预约拦截
	book(t, b, u1, other.ID, p1.ID)
	if _, err := b.CreateBooking(context.Background(), u1, other.ID, p1.ID); err == nil {
		t.Fatal("同一宠物同一时段重复预约应拦截")
	}
	// 同宠物重叠时段拦截
	if _, err := b.CreateBooking(context.Background(), u1, other.ID, p1.ID); err == nil || err.(*errs.BizError).Code != errs.ErrPetConflict.Code {
		t.Fatalf("重叠时段应拦截: %v", err)
	}
	// 另一只宠物不受影响
	if _, err := b.CreateBooking(context.Background(), u2, other.ID, p2.ID); err != nil {
		t.Fatalf("其他宠物同时段应可预约: %v", err)
	}
}

// TestCreateBookingConcurrent 并发争抢最后名额：仅 1 个成功。
func TestCreateBookingConcurrent(t *testing.T) {
	b := newBiz(t)
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	var users []*model.User
	var pets []*model.Pet
	for i := 0; i < 20; i++ {
		u := mkUser(t, b, fmt.Sprintf("c%d", i), "pet_owner", 0)
		users = append(users, u)
		pets = append(pets, mkPet(t, b, u.ID, fmt.Sprintf("宠%d", i)))
	}
	var mu sync.Mutex
	success, fail := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := b.CreateBooking(context.Background(), users[i], slot.ID, pets[i].ID)
			mu.Lock()
			if err == nil {
				success++
			} else {
				fail++
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if success != 3 || fail != 17 {
		t.Fatalf("并发抢 3 名额应恰好 3 成功 17 失败: success=%d fail=%d", success, fail)
	}
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 0 {
		t.Fatalf("名额应扣完: %d", s.Remaining)
	}
	var cnt int64
	b.DB.Model(&model.Appointment{}).Where("slot_id = ?", slot.ID).Count(&cnt)
	if cnt != 3 {
		t.Fatalf("有效订单应为 3: %d", cnt)
	}
}

// ---------- 待支付超时与主动取消 ----------

func TestPendingExpiryReleasesSlot(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)

	// 拨快 6 分钟触发超时
	if err := b.Clock.SetOffset(6 * 60); err != nil {
		t.Fatal(err)
	}
	b.expireDuePending()
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 3 {
		t.Fatalf("超时关单应释放名额: %d", s.Remaining)
	}
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.Status != model.StatusClosed {
		t.Fatalf("应为已关闭: %s", ap2.Status)
	}
	// 幂等：再次关单不多释放
	b.expireDuePending()
	b.DB.First(&s, slot.ID)
	if s.Remaining != 3 {
		t.Fatalf("重复关单不得重复释放: %d", s.Remaining)
	}
	b.Clock.SetOffset(0)
}

func TestCancelUnpaid(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	u2 := mkUser(t, b, "user2", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)

	if _, err := b.CancelUnpaid(context.Background(), u2, ap.ID); err == nil {
		t.Fatal("不能取消他人订单")
	}
	msg, err := b.CancelUnpaid(context.Background(), u, ap.ID)
	if err != nil {
		t.Fatalf("主动取消: %v", err)
	}
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 3 || msg == "" {
		t.Fatalf("取消后应释放名额: remaining=%d msg=%q", s.Remaining, msg)
	}
	// 幂等
	msg2, err := b.CancelUnpaid(context.Background(), u, ap.ID)
	if err != nil || msg2 == "" {
		t.Fatalf("重复取消应幂等返回成功提示: %v %q", err, msg2)
	}
	b.DB.First(&s, slot.ID)
	if s.Remaining != 3 {
		t.Fatalf("重复取消不得重复释放: %d", s.Remaining)
	}
}

// ---------- 支付通知 ----------

func payNotify(b *Biz, notifyID, orderNo, tradeNo string, amount int64, result string) (string, error) {
	return b.PayNotify(context.Background(), notifyID, orderNo, tradeNo, amount, result,
		sign.Pay(notifyID, orderNo, tradeNo, amount, result))
}

func TestPayNotifyConfirm(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)

	// 验签失败
	if _, err := b.PayNotify(context.Background(), "N1", ap.OrderNo, "T1", 9900, "success", "badsig"); err == nil {
		t.Fatal("验签失败应拒绝")
	}
	// 金额不符
	if _, err := payNotify(b, "N2", ap.OrderNo, "T2", 100, "success"); err == nil {
		t.Fatal("金额不符应拒绝")
	}
	var cnt int64
	b.DB.Model(&model.PaymentRecord{}).Where("kind = ?", model.PayKindRejected).Count(&cnt)
	if cnt == 0 {
		t.Fatal("被拒绝的通知应留痕可查")
	}
	// 有效支付
	msg, err := payNotify(b, "N3", ap.OrderNo, "T3", 9900, "success")
	if err != nil || msg == "" {
		t.Fatalf("有效支付: %v %q", err, msg)
	}
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.Status != model.StatusConfirmed {
		t.Fatalf("支付成功应确认预约: %s", ap2.Status)
	}
	// 重复通知（同 notifyId）不重复确认
	msg2, _ := payNotify(b, "N3", ap.OrderNo, "T3", 9900, "success")
	if msg2 == "" {
		t.Fatal("重复通知应有幂等提示")
	}
	var n int64
	b.DB.Model(&model.PaymentRecord{}).Where("notify_id = ?", "N3").Count(&n)
	if n != 1 {
		t.Fatalf("重复通知不得产生新流水: %d", n)
	}
	// 同订单第二笔成功支付（不同交易号）→ 异常支付
	_, _ = payNotify(b, "N4", ap.OrderNo, "T4", 9900, "success")
	var ab model.AbnormalPayment
	if err := b.DB.Where("channel_trade_no = ?", "T4").First(&ab).Error; err != nil {
		t.Fatalf("第二笔支付应划入异常支付: %v", err)
	}
	if ab.Reason != model.PayKindDup {
		t.Fatalf("异常原因应为重复支付: %s", ab.Reason)
	}
	var ap3 model.Appointment
	b.DB.First(&ap3, ap.ID)
	if ap3.Status != model.StatusConfirmed {
		t.Fatalf("重复支付通知不得重复确认/改变状态: %s", ap3.Status)
	}
}

func TestLatePaymentGoesAbnormal(t *testing.T) {
	b := newBiz(t)
	u := mkUser(t, b, "user1", "pet_owner", 0)
	pet := mkPet(t, b, u.ID, "豆包")
	start := b.Clock.Now().Add(26 * time.Hour)
	slot := mkSlot(t, b, start, start.Add(time.Hour), 3)
	ap := book(t, b, u, slot.ID, pet.ID)
	// 关单（主动取消）
	_, _ = b.CancelUnpaid(context.Background(), u, ap.ID)
	// 迟到支付成功
	msg, err := payNotify(b, "L1", ap.OrderNo, "TL1", 9900, "success")
	if err != nil || msg == "" {
		t.Fatalf("迟到支付通知应受理并登记异常: %v %q", err, msg)
	}
	var ab model.AbnormalPayment
	b.DB.Where("channel_trade_no = ?", "TL1").First(&ab)
	if ab.Reason != "closed_order_paid" || ab.Status != model.AbnormalPending {
		t.Fatalf("迟到支付应进入异常支付待处理: %+v", ab)
	}
	var ap2 model.Appointment
	b.DB.First(&ap2, ap.ID)
	if ap2.Status != model.StatusClosed {
		t.Fatalf("迟到支付不得恢复预约: %s", ap2.Status)
	}
	var s model.TimeSlot
	b.DB.First(&s, slot.ID)
	if s.Remaining != 3 {
		t.Fatalf("迟到支付不得返还名额: %d", s.Remaining)
	}
	_ = msg
}

func TestPayNotifyUnknownOrderRejected(t *testing.T) {
	b := newBiz(t)
	_, err := payNotify(b, "X1", "B999999999999999999", "TX1", 9900, "success")
	if err == nil {
		t.Fatal("未知订单应拒绝")
	}
	var rec model.PaymentRecord
	if e := b.DB.Where("notify_id = ?", "X1").First(&rec).Error; e != nil || rec.Kind != model.PayKindRejected {
		t.Fatalf("未知订单通知应留痕: %v %+v", e, rec)
	}
}
