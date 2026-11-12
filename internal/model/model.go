// Package model 定义 GORM 数据模型与预约状态常量。
package model

import "time"

// 预约单状态。
const (
	StatusPendingPayment = "pending_payment" // 待支付（预扣名额）
	StatusClosed         = "closed"          // 已关闭（超时/主动取消/退款终态后不再复活）
	StatusConfirmed      = "confirmed"       // 已确认预约
	StatusCancelled      = "cancelled"       // 已取消
	StatusCheckedIn      = "checked_in"      // 已核销
	StatusInService      = "in_service"      // 服务中
	StatusCompleted      = "completed"       // 已完成
	StatusNoShow         = "no_show"         // 未到店
)

// 有效预约状态 = 待支付、已支付（占用名额与宠物时段锁）。
func EffectiveStatuses() []string {
	return []string{StatusPendingPayment, StatusConfirmed}
}

// 取消申请状态。
const (
	CancelPending  = "pending"
	CancelApproved = "approved"
	CancelRejected = "rejected"
)

// 支付记录结果。
const (
	PayResultSuccess = "success"
	PayResultFail    = "fail"
)

// 支付记录归类。
const (
	PayKindNormal   = "normal"   // 有效支付
	PayKindLate     = "late"     // 关单后迟到支付 → 异常支付
	PayKindDup      = "dup"      // 同订单重复支付 → 异常支付
	PayKindRejected = "rejected" // 金额不符/验签失败等
)

// 退款状态。
const (
	RefundProcessing = "processing"
	RefundSuccess    = "success"
	RefundFailed     = "failed"       // 单次失败，可重试
	RefundFinalFail  = "final_failed" // 重试 3 次耗尽
	RefundTimeout    = "timeout"      // 7 自然日回调超时（仍在处理中口径）
)

// 退款类别。
const (
	RefundKindNormal   = "normal"   // 取消退款
	RefundKindAbnormal = "abnormal" // 异常支付退款
)

// 异常支付状态。
const (
	AbnormalPending   = "pending"
	AbnormalRefunding = "refunding"
	AbnormalRefunded  = "refunded"
)

// 时段状态。
const (
	SlotOpen   = "open"
	SlotClosed = "closed" // 手动关闭/过期
)

// User 用户与角色。role: owner / pet_owner / front / support。
type User struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"size:32;uniqueIndex" json:"username"`
	Role      string    `gorm:"size:16;index" json:"role"`
	ShopID    int64     `json:"shopId"` // 前台绑定的门店 ID，其余角色为 0
	CreatedAt time.Time `json:"createdAt"`
}

// Shop 门店。
type Shop struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64" json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// ServiceItem 服务项目（洗护）。
type ServiceItem struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	ShopID    int64     `gorm:"index" json:"shopId"`
	Name      string    `gorm:"size:64" json:"name"`
	PriceFen  int64     `json:"priceFen"`
	CreatedAt time.Time `json:"createdAt"`
}

// Pet 宠物（只读档案）。
type Pet struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	UserID    int64     `gorm:"index" json:"userId"`
	Name      string    `gorm:"size:32" json:"name"`
	Breed     string    `gorm:"size:32" json:"breed"`
	CreatedAt time.Time `json:"createdAt"`
}

// TimeSlot 可预约时段。名额全局共享。
type TimeSlot struct {
	ID            int64     `gorm:"primaryKey" json:"id"`
	ShopID        int64     `gorm:"index" json:"shopId"`
	ServiceItemID int64     `json:"serviceItemId"`
	StartTime     time.Time `gorm:"index" json:"startTime"`
	EndTime       time.Time `json:"endTime"`
	Capacity      int32     `json:"capacity"`
	Remaining     int32     `json:"remaining"`
	Status        string    `gorm:"size:16;default:open" json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Appointment 预约单（= 订单）。
type Appointment struct {
	ID           int64      `gorm:"primaryKey" json:"id"`
	OrderNo      string     `gorm:"size:32;uniqueIndex" json:"orderNo"`
	UserID       int64      `gorm:"index" json:"userId"`
	PetID        int64      `gorm:"index" json:"petId"`
	SlotID       int64      `gorm:"index" json:"slotId"`
	ShopID       int64      `gorm:"index" json:"shopId"`
	AmountFen    int64      `json:"amountFen"`
	Status       string     `gorm:"size:24;index" json:"status"`
	PayDeadline  time.Time  `json:"payDeadline"`  // 待支付计时起点 + 5 分钟
	SlotReleased bool       `json:"slotReleased"` // 取消退款成功后名额已原路释放（幂等标记）
	CheckedInAt  *time.Time `json:"checkedInAt"`
	StartAt      *time.Time `json:"startAt"`
	CompletedAt  *time.Time `json:"completedAt"`
	NoShowReason string     `gorm:"size:128" json:"noShowReason"`
	NoShowAt     *time.Time `json:"noShowAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// PaymentRecord 支付通知流水。幂等键 = (订单号, 通知号)：
// 渠道进程重启后通知号可能复用（如 N1-1），不能按全局 notify_id 唯一约束。
type PaymentRecord struct {
	ID             int64     `gorm:"primaryKey" json:"id"`
	NotifyID       string    `gorm:"size:64;uniqueIndex:idx_payment_order_notify;index" json:"notifyId"`
	OrderNo        string    `gorm:"size:32;uniqueIndex:idx_payment_order_notify;index" json:"orderNo"`
	ChannelTradeNo string    `gorm:"size:64;index" json:"channelTradeNo"`
	AmountFen      int64     `json:"amountFen"`
	Result         string    `gorm:"size:16" json:"result"`
	Kind           string    `gorm:"size:32" json:"kind"` // normal/late/dup/rejected/closed_order_paid
	Detail         string    `gorm:"size:255" json:"detail"`
	CreatedAt      time.Time `json:"createdAt"`
}

// Refund 退款单。退款状态独立于预约状态。
type Refund struct {
	ID         int64     `gorm:"primaryKey" json:"id"`
	RefundNo   string    `gorm:"size:32;uniqueIndex" json:"refundNo"`
	BookingID  int64     `gorm:"index" json:"bookingId"` // 0 表示异常支付退款
	OrderNo    string    `gorm:"size:32;index" json:"orderNo"`
	AbnormalID int64     `json:"abnormalId"`
	AmountFen  int64     `json:"amountFen"`
	Status     string    `gorm:"size:24;index" json:"status"`
	RetryCount int32     `json:"retryCount"`
	FailReason string    `gorm:"size:255" json:"failReason"`
	Kind       string    `gorm:"size:16" json:"kind"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// CancelRequest 取消申请。
type CancelRequest struct {
	ID           int64      `gorm:"primaryKey" json:"id"`
	BookingID    int64      `gorm:"index" json:"bookingId"`
	UserID       int64      `json:"userId"`
	Reason       string     `gorm:"size:255" json:"reason"`
	Status       string     `gorm:"size:16;index" json:"status"`
	ReviewReason string     `gorm:"size:255" json:"reviewReason"`
	Urgent       bool       `json:"urgent"` // 距开始 <2h
	CreatedAt    time.Time  `json:"createdAt"`
	ReviewedAt   *time.Time `json:"reviewedAt"`
}

// AbnormalPayment 异常支付台账（永久保存）。
type AbnormalPayment struct {
	ID             int64     `gorm:"primaryKey" json:"id"`
	OrderNo        string    `gorm:"size:32;index" json:"orderNo"`
	ChannelTradeNo string    `gorm:"size:64;uniqueIndex" json:"channelTradeNo"`
	AmountFen      int64     `json:"amountFen"`
	Reason         string    `gorm:"size:64" json:"reason"` // closed_order_paid / duplicate_payment
	Status         string    `gorm:"size:16" json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
}

// AuditLog 操作审计（核销/取消/审核/标记等关键动作）。
type AuditLog struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	ActorID   int64     `gorm:"index" json:"actorId"`
	Role      string    `gorm:"size:16" json:"role"`
	Action    string    `gorm:"size:32;index" json:"action"`
	TargetID  int64     `json:"targetId"`
	Detail    string    `gorm:"size:255" json:"detail"`
	CreatedAt time.Time `json:"createdAt"`
}

// ClockOffset 教学时钟偏移（单行 id=1）。
type ClockOffset struct {
	ID        int64 `gorm:"primaryKey"`
	OffsetSec int64
	UpdatedAt time.Time
}

// TableName 自定义表名。
func (ClockOffset) TableName() string { return "clock_offsets" }
