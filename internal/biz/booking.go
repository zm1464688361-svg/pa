package biz

import (
	"context"
	"strconv"
	"time"

	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/repo"
	"gorm.io/gorm"
)

// ---------- 时段 ----------

// CreateSlot 店长发布可预约时段：必须明确起止时间与名额。
func (b *Biz) CreateSlot(ctx context.Context, actorID int64, role string, startStr, endStr string, capacity int32) (*model.TimeSlot, error) {
	if err := b.requireRole(role, "owner"); err != nil {
		return nil, err
	}
	start, err1 := time.ParseInLocation("2006-01-02 15:04", startStr, time.Local)
	end, err2 := time.ParseInLocation("2006-01-02 15:04", endStr, time.Local)
	if err1 != nil || err2 != nil || !end.After(start) {
		return nil, errs.ErrSlotTimeBad
	}
	if capacity <= 0 {
		return nil, errs.ErrSlotCapacity
	}
	var item model.ServiceItem
	if err := b.DB.First(&item).Error; err != nil {
		return nil, errs.ErrNotFound.With("服务项目")
	}
	s := model.TimeSlot{
		ShopID: item.ShopID, ServiceItemID: item.ID,
		StartTime: start, EndTime: end,
		Capacity: capacity, Remaining: capacity, Status: model.SlotOpen,
	}
	if err := b.DB.Create(&s).Error; err != nil {
		return nil, err
	}
	repo.WriteAudit(b.DB, actorID, role, "create_slot", s.ID,
		"发布时段 "+slotRange(&s)+" 名额 "+itoa(int64(capacity)))
	return &s, nil
}

// SlotView 时段列表项（含可预约性判断）。
type SlotView struct {
	Slot     *model.TimeSlot
	Bookable bool
	Reason   string // 不可预约原因
}

// ListSlots 查询时段（店长/主人/前台均可，权限矩阵允许）。列表前先惰性关单。
func (b *Biz) ListSlots(ctx context.Context, role string) ([]SlotView, *model.ServiceItem, error) {
	if err := b.requireRole(role, "owner", "pet_owner", "front"); err != nil {
		return nil, nil, err
	}
	b.expireDuePending()
	var item model.ServiceItem
	if err := b.DB.First(&item).Error; err != nil {
		return nil, nil, errs.ErrNotFound.With("服务项目")
	}
	var slots []model.TimeSlot
	if err := b.DB.Order("start_time asc").Find(&slots).Error; err != nil {
		return nil, nil, err
	}
	now := b.Clock.Now()
	views := make([]SlotView, 0, len(slots))
	for i := range slots {
		s := &slots[i]
		v := SlotView{Slot: s, Bookable: true}
		// 时段过期自动关闭（惰性落库一次）
		if now.After(s.EndTime) && s.Status == model.SlotOpen {
			if err := b.DB.Model(s).UpdateColumn("status", model.SlotClosed).Error; err == nil {
				s.Status = model.SlotClosed
			}
		}
		switch {
		case s.Status != model.SlotOpen:
			v.Bookable, v.Reason = false, "已关闭"
		case now.After(s.StartTime) || now.Equal(s.StartTime):
			v.Bookable, v.Reason = false, "时段已开始，不可预约"
		case s.Remaining <= 0:
			v.Bookable, v.Reason = false, "名额已满"
		}
		views = append(views, v)
	}
	return views, &item, nil
}

// ---------- 预约 ----------

// CreateBooking 主人选宠提交预约：前置强制校验 + 原子预扣名额 + 宠物互斥锁。
func (b *Biz) CreateBooking(ctx context.Context, actor *model.User, slotID, petID int64) (*model.Appointment, error) {
	if err := b.requireRole(actor.Role, "pet_owner"); err != nil {
		return nil, err
	}
	var ap *model.Appointment
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		// 1) 锁时段行
		s, err := b.getSlot(tx, slotID)
		if err != nil {
			return err
		}
		now := b.Clock.Now()
		if now.After(s.EndTime) {
			return errs.ErrSlotExpired
		}
		if now.After(s.StartTime) || now.Equal(s.StartTime) {
			return errs.ErrSlotStarted
		}
		if s.Status != model.SlotOpen {
			return errs.ErrSlotExpired
		}
		if s.Remaining <= 0 {
			return errs.ErrSlotFull
		}
		// 2) 宠物归属校验
		var pet model.Pet
		if err := tx.Where("id = ? AND user_id = ?", petID, actor.ID).First(&pet).Error; err != nil {
			return errs.ErrPetNotYours
		}
		// 3) 宠物互斥锁：先惰性关掉本人到期未支付单，再查重叠有效预约
		var mine []model.Appointment
		tx.Where("pet_id = ? AND status = ?", petID, model.StatusPendingPayment).Find(&mine)
		for i := range mine {
			b.expireIfDue(tx, &mine[i])
		}
		var conflicts int64
		err = tx.Model(&model.Appointment{}).
			Joins("JOIN time_slots s ON s.id = appointments.slot_id").
			Where("appointments.pet_id = ? AND appointments.status IN ? AND s.shop_id = ? AND s.start_time < ? AND s.end_time > ?",
				petID, model.EffectiveStatuses(), s.ShopID, s.EndTime, s.StartTime).
			Count(&conflicts).Error
		if err != nil {
			return err
		}
		if conflicts > 0 {
			return errs.ErrPetConflict
		}
		// 4) 原子预扣名额（行锁已持有，条件更新兜底并发）
		res := tx.Model(&model.TimeSlot{}).
			Where("id = ? AND remaining > 0", slotID).
			UpdateColumn("remaining", gorm.Expr("remaining - 1"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.ErrSlotFull
		}
		// 5) 生成待支付订单（占用名额 5 分钟，计时起点=订单创建时间）
		ap = &model.Appointment{
			OrderNo: genOrderNo(), UserID: actor.ID, PetID: petID, SlotID: slotID,
			ShopID: s.ShopID, AmountFen: 0, Status: model.StatusPendingPayment,
			PayDeadline: now.Add(payHoldDuration), CreatedAt: now, UpdatedAt: now,
		}
		var item model.ServiceItem
		if err := tx.First(&item, s.ServiceItemID).Error; err != nil {
			return err
		}
		ap.AmountFen = item.PriceFen
		if err := tx.Create(ap).Error; err != nil {
			return err
		}
		repo.WriteAudit(tx, actor.ID, actor.Role, "create_booking", ap.ID,
			pet.Name+" 预约 "+slotRange(s))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ap, nil
}

// CancelUnpaid 主人主动取消待支付订单：关单并释放名额（幂等）。
func (b *Biz) CancelUnpaid(ctx context.Context, actor *model.User, bookingID int64) (string, error) {
	if err := b.requireRole(actor.Role, "pet_owner"); err != nil {
		return "", err
	}
	msg := ""
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		ap, err := b.getBooking(tx, bookingID)
		if err != nil {
			return err
		}
		if ap.UserID != actor.ID {
			return errs.ErrNotYourBooking
		}
		if ap.Status == model.StatusClosed {
			msg = "订单已关闭（此前已释放名额）"
			return nil // 幂等：重复取消返回成功提示
		}
		if ap.Status != model.StatusPendingPayment {
			return errs.ErrStateInvalid.With("当前状态 %s", statusText(ap.Status))
		}
		res := tx.Model(&model.Appointment{}).
			Where("id = ? AND status = ?", ap.ID, model.StatusPendingPayment).
			Updates(map[string]interface{}{"status": model.StatusClosed, "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			b.releaseSlot(tx, ap.SlotID)
			msg = "订单已取消，名额已释放"
			repo.WriteAudit(tx, actor.ID, actor.Role, "cancel_unpaid", ap.ID, "主动取消待支付订单 "+ap.OrderNo)
		} else {
			msg = "订单已关闭（此前已释放名额）"
		}
		return nil
	})
	return msg, err
}

// ListBookings 按角色查询预约列表。主人仅本人；店长/前台本店全部。
func (b *Biz) ListBookings(ctx context.Context, actor *model.User, slotID int64) ([]BookingRow, error) {
	b.expireDuePending()
	q := b.DB.Model(&model.Appointment{}).
		Select("appointments.*, pets.name AS pet_name, users.username AS user_name, ts.start_time AS slot_start, ts.end_time AS slot_end").
		Joins("JOIN pets ON pets.id = appointments.pet_id").
		Joins("JOIN users ON users.id = appointments.user_id").
		Joins("JOIN time_slots ts ON ts.id = appointments.slot_id").
		Order("appointments.id desc")
	switch actor.Role {
	case "pet_owner":
		q = q.Where("appointments.user_id = ?", actor.ID)
	case "owner", "front":
		shopID := actor.ShopID
		if shopID == 0 {
			shopID = 1
		}
		q = q.Where("appointments.shop_id = ?", shopID)
	case "support":
		// 客服不在此页查看全部预约
		return nil, errs.ErrForbidden
	default:
		return nil, errs.ErrForbidden
	}
	if slotID > 0 {
		q = q.Where("appointments.slot_id = ?", slotID)
	}
	var rows []bookingJoin
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]BookingRow, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		// 退款处理中提示
		status := statusText(r.Status)
		if r.Status == model.StatusCancelled && !r.SlotReleased {
			status = "已取消 · 退款处理中"
		}
		out = append(out, BookingRow{
			Ap: toAp(&r.Appointment), PetName: r.PetName, UserName: r.UserName,
			SlotStart: r.SlotStart, SlotEnd: r.SlotEnd, StatusText: status,
		})
	}
	return out, nil
}

// BookingRow 列表展示行。
type BookingRow struct {
	Ap         *model.Appointment
	PetName    string
	UserName   string
	SlotStart  time.Time
	SlotEnd    time.Time
	StatusText string
}

// GetBookingDetail 权限受限的预约详情。
func (b *Biz) GetBookingDetail(ctx context.Context, actor *model.User, bookingID int64) (*model.Appointment, string, error) {
	b.expireDuePending()
	var ap model.Appointment
	if err := b.DB.First(&ap, bookingID).Error; err != nil {
		return nil, "", errs.ErrBookingNotFound
	}
	switch actor.Role {
	case "pet_owner":
		if ap.UserID != actor.ID {
			return nil, "", errs.ErrNotYourBooking
		}
	case "owner", "front":
		shopID := actor.ShopID
		if shopID == 0 {
			shopID = 1
		}
		if ap.ShopID != shopID {
			return nil, "", errs.ErrForbidden
		}
	case "support":
		// 客服可经取消审核/退款列表进入，允许查看
	default:
		return nil, "", errs.ErrForbidden
	}
	status := statusText(ap.Status)
	if ap.Status == model.StatusCancelled && !ap.SlotReleased {
		status = "已取消 · 退款处理中"
	}
	return &ap, status, nil
}

// HasPendingCancel 预约是否存在审核中取消申请（核销前置校验使用）。
func (b *Biz) HasPendingCancel(tx *gorm.DB, bookingID int64) (bool, error) {
	var n int64
	err := tx.Model(&model.CancelRequest{}).
		Where("booking_id = ? AND status = ?", bookingID, model.CancelPending).
		Count(&n).Error
	return n > 0, err
}

func toAp(a *model.Appointment) *model.Appointment { cp := *a; return &cp }

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// bookingJoin 列表联查结果。
type bookingJoin struct {
	model.Appointment
	PetName   string
	UserName  string
	SlotStart time.Time
	SlotEnd   time.Time
}
