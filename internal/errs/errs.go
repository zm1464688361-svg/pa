// Package errs 定义业务错误。Code 用于 RPC 与 HTTP 返回，Msg 为用户可见文案。
package errs

import "fmt"

// BizError 业务错误，带稳定 Code。
type BizError struct {
	Code int
	Msg  string
}

func (e *BizError) Error() string { return fmt.Sprintf("[%d] %s", e.Code, e.Msg) }

func New(code int, msg string) *BizError { return &BizError{Code: code, Msg: msg} }

func (e *BizError) With(format string, args ...interface{}) *BizError {
	return &BizError{Code: e.Code, Msg: e.Msg + ": " + fmt.Sprintf(format, args...)}
}

// 错误码分段：1xxx 通用/权限，2xxx 时段，3xxx 预约，4xxx 支付，5xxx 取消退款，6xxx 核销履约，7xxx 未到店，8xxx 报表。
var (
	ErrForbidden    = New(1001, "无权执行该操作")
	ErrNotFound     = New(1002, "记录不存在")
	ErrBadRequest   = New(1003, "请求参数不合法")
	ErrAuthFailed   = New(1004, "鉴权失败")
	ErrAlreadyDone  = New(1005, "重复操作，结果与首次一致")
	ErrStateInvalid = New(1006, "当前状态不允许该操作")

	ErrSlotNotFound = New(2001, "时段不存在")
	ErrSlotTimeBad  = New(2002, "时段起止时间不合法（需明确开始/结束时间，且结束晚于开始）")
	ErrSlotCapacity = New(2003, "时段名额必须大于 0")
	ErrSlotFull     = New(2004, "该时段名额已满")
	ErrSlotStarted  = New(2005, "该时段已开始，不可预约")
	ErrSlotExpired  = New(2006, "该时段已过期关闭")
	ErrSlotNotEnded = New(2007, "时段尚未结束")

	ErrPetNotYours     = New(3001, "只能为本人名下的宠物预约")
	ErrPetConflict     = New(3002, "该宠物在重叠时段已有有效预约")
	ErrBookingNotFound = New(3003, "预约单不存在")
	ErrNotYourBooking  = New(3004, "只能操作本人的预约单")

	ErrPayAmount     = New(4001, "支付金额与订单不符")
	ErrPayClosed     = New(4002, "订单已关闭，支付通知进入异常支付处理")
	ErrPayUnknown    = New(4003, "支付通知对应的订单不存在")
	ErrPaySign       = New(4004, "支付通知验签失败")
	ErrPayDupInvalid = New(4005, "重复支付通知，已按首次结果处理")

	ErrCancelReasonRequired  = New(5001, "取消原因必填，空内容不允许提交")
	ErrCancelNotCancellable  = New(5002, "当前状态不可取消（已核销/服务中/已完成的预约不能取消退款）")
	ErrCancelRequestNotFound = New(5003, "取消申请不存在")
	ErrCancelAlreadyReviewed = New(5004, "该申请已审核过，不能重复审核")

	ErrCheckInForbidden   = New(6001, "核销被拒绝：仅可核销本店、已确认且无审核中取消申请的预约")
	ErrCheckInSkip        = New(6002, "流程顺序错误：须先核销再登记服务开始/完成")
	ErrStartNotCheckedIn  = New(6003, "须先核销，才能登记服务开始")
	ErrCompleteNotStarted = New(6004, "须先登记服务开始，才能登记服务完成")

	ErrNoShowNotEnded = New(7001, "时段结束时间未到达，不能标记未到店")
	ErrNoShowDone     = New(7002, "该预约已标记未到店，标记状态不可回退")
	ErrNoShowReason   = New(7003, "未到店原因必填")

	ErrRefundNotFound  = New(8001, "退款单不存在")
	ErrRefundRetried   = New(8002, "退款已达最大重试次数（3 次），标记终态失败")
	ErrReportForbidden = New(8003, "仅店长可查看经营数据")
)
