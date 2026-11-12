namespace go petshop

// ---------- 公共 ----------
struct Actor {
    1: i64 userId,
    2: string role,      // owner / pet_owner / front / support
    3: i64 shopId,       // 前台绑定门店
}

struct BaseResp {
    1: i32 code,
    2: string msg,
}

// ---------- 时段 ----------
struct TimeSlot {
    1: i64 id,
    2: string startTime,
    3: string endTime,
    4: i32 capacity,
    5: i32 remaining,
    6: string status,      // open / closed / expired
}
struct SlotView {
    1: TimeSlot slot,
    2: bool bookable,
    3: string reason,      // 不可预约原因
}
struct CreateSlotReq {
    1: Actor actor,
    2: string startTime,
    3: string endTime,
    4: i32 capacity,
}
struct CreateSlotResp {
    1: BaseResp base,
    2: TimeSlot slot,
}
struct ListSlotsReq {
    1: Actor actor,
}
struct ListSlotsResp {
    1: BaseResp base,
    2: list<SlotView> slots,
    3: string itemName,
    4: i64 priceFen,
}

// ---------- 预约 ----------
struct Booking {
    1: i64 id,
    2: string orderNo,
    3: i64 userId,
    4: string userName,
    5: i64 petId,
    6: string petName,
    7: i64 slotId,
    8: string slotStart,
    9: string slotEnd,
    10: string status,
    11: i64 amountFen,
    12: string payDeadline,
    13: string checkinCode,
    14: string cancelReason,
    15: string rejectReason,
    16: string createdAt,
    17: string noShowReason,
    18: string statusText,
}
struct CreateBookingReq {
    1: Actor actor,
    2: i64 slotId,
    3: i64 petId,
}
struct CreateBookingResp {
    1: BaseResp base,
    2: Booking booking,
}
struct GetBookingsReq {
    1: Actor actor,
    2: i64 slotId,        // 0 = 全部
}
struct GetBookingsResp {
    1: BaseResp base,
    2: list<Booking> bookings,
}
struct GetBookingDetailReq {
    1: Actor actor,
    2: i64 bookingId,
}
struct GetBookingDetailResp {
    1: BaseResp base,
    2: Booking booking,
}
struct CancelUnpaidReq {
    1: Actor actor,
    2: i64 bookingId,
}
struct CancelUnpaidResp {
    1: BaseResp base,
    2: string msg,
}

// ---------- 取消申请 ----------
struct CancelRequest {
    1: i64 id,
    2: i64 bookingId,
    3: string reason,
    4: string status,      // pending / approved / rejected
    5: string reviewReason,
    6: bool urgent,        // <2h
    7: string createdAt,
}
struct CancelRequestView {
    1: CancelRequest request,
    2: string orderNo,
    3: string userPet,
    4: string slotRange,
    5: string statusText,
    6: i64 amountFen,
}
struct SubmitCancelReq {
    1: Actor actor,
    2: i64 bookingId,
    3: string reason,
}
struct SubmitCancelResp {
    1: BaseResp base,
    2: CancelRequest request,
    3: string msg,
}
struct ListCancelRequestsReq {
    1: Actor actor,
    2: string status,      // 空=全部
}
struct ListCancelRequestsResp {
    1: BaseResp base,
    2: list<CancelRequestView> requests,
}
struct ReviewCancelReq {
    1: Actor actor,
    2: i64 requestId,
    3: bool approve,
    4: string reason,
}
struct ReviewCancelResp {
    1: BaseResp base,
    2: string msg,
}

// ---------- 支付回调（pay_sim -> gateway -> service） ----------
struct PayNotifyReq {
    1: string notifyId,      // 渠道通知唯一号（幂等键）
    2: string orderNo,
    3: string channelTradeNo,
    4: i64 amountFen,
    5: string payResult,     // success / fail
    6: string sign,
}
struct PayNotifyResp {
    1: BaseResp base,
    2: string msg,
}

// ---------- 退款 ----------
struct Refund {
    1: i64 id,
    2: string refundNo,
    3: i64 bookingId,
    4: string orderNo,
    5: i64 amountFen,
    6: string status,
    7: i32 retryCount,
    8: string failReason,
    9: string kind,          // normal / abnormal
    10: string createdAt,
    11: string statusText,
    12: bool timeoutFlag,
}
struct ListRefundsReq {
    1: Actor actor,
    2: string status,
}
struct ListRefundsResp {
    1: BaseResp base,
    2: list<Refund> refunds,
}
struct RetryRefundReq {
    1: Actor actor,
    2: i64 refundId,
}
struct RetryRefundResp {
    1: BaseResp base,
    2: string msg,
}
struct RefundNotifyReq {
    1: string notifyId,
    2: string refundNo,
    3: string payResult,     // success / fail
    4: string sign,
}
struct RefundNotifyResp {
    1: BaseResp base,
    2: string msg,
}

// ---------- 异常支付 ----------
struct AbnormalPayment {
    1: i64 id,
    2: string orderNo,
    3: string channelTradeNo,
    4: i64 amountFen,
    5: string reason,        // closed_order_paid / duplicate_payment
    6: string status,        // pending / refunding / refunded
    7: string createdAt,
    8: string statusText,
}
struct ListAbnormalReq {
    1: Actor actor,
}
struct ListAbnormalResp {
    1: BaseResp base,
    2: list<AbnormalPayment> items,
}
struct RefundAbnormalReq {
    1: Actor actor,
    2: i64 abnormalId,
}
struct RefundAbnormalResp {
    1: BaseResp base,
    2: string msg,
}

// ---------- 履约 ----------
struct CheckInReq {
    1: Actor actor,
    2: i64 bookingId,
}
struct CheckInResp {
    1: BaseResp base,
    2: string msg,
}
struct ServiceProgressReq {
    1: Actor actor,
    2: i64 bookingId,
    3: string action,        // start / complete
}
struct ServiceProgressResp {
    1: BaseResp base,
    2: string msg,
}

// ---------- 未到店 ----------
struct MarkNoShowReq {
    1: Actor actor,
    2: i64 bookingId,
    3: string reason,
}
struct MarkNoShowResp {
    1: BaseResp base,
    2: string msg,
}
struct UpdateNoShowReasonReq {
    1: Actor actor,
    2: i64 bookingId,
    3: string reason,
}
struct UpdateNoShowReasonResp {
    1: BaseResp base,
    2: string msg,
}

// ---------- 日清报表 ----------
struct SlotRow {
    1: string slotRange,
    2: i32 validCount,
    3: i32 completedCount,
    4: i32 cancelledCount,
    5: i32 noShowCount,
    6: i32 refundPendingCount,
}
struct RecordRow {
    1: string time,
    2: string orderNo,
    3: string userPet,
    4: string slotRange,
    5: string statusText,
    6: string detail,
    7: i64 amountFen,
}
struct DailyReportReq {
    1: Actor actor,
    2: string date,          // yyyy-MM-dd
}
struct DailyReportResp {
    1: BaseResp base,
    2: i64 receivedFen,      // 已收款
    3: i64 refundedFen,      // 已退款
    4: i64 netFen,           // 实收
    5: i64 refundPendingFen, // 退款待跟进
    6: list<SlotRow> slotRows,
    7: list<RecordRow> records,
    8: i64 completedFen,     // 已完成服务收入（未到店不计入）
    9: i32 noShowCount,      // 未到店单数（单独统计）
    10: i64 noShowFen,       // 未到店金额（已收款未退款）
}

// ---------- 教学时钟 ----------
struct GetClockResp {
    1: BaseResp base,
    2: string now,
    3: i64 offsetSec,
}
struct SetClockReq {
    1: i64 offsetSec,
}
struct SetClockResp {
    1: BaseResp base,
}

service AppointmentService {
    // 时段
    CreateSlotResp CreateSlot(1: CreateSlotReq req)
    ListSlotsResp ListSlots(1: ListSlotsReq req)
    // 预约
    CreateBookingResp CreateBooking(1: CreateBookingReq req)
    GetBookingsResp GetBookings(1: GetBookingsReq req)
    GetBookingDetailResp GetBookingDetail(1: GetBookingDetailReq req)
    CancelUnpaidResp CancelUnpaid(1: CancelUnpaidReq req)
    // 取消申请
    SubmitCancelResp SubmitCancel(1: SubmitCancelReq req)
    ListCancelRequestsResp ListCancelRequests(1: ListCancelRequestsReq req)
    ReviewCancelResp ReviewCancel(1: ReviewCancelReq req)
    // 支付/退款回调
    PayNotifyResp PayNotify(1: PayNotifyReq req)
    RefundNotifyResp RefundNotify(1: RefundNotifyReq req)
    // 退款与异常
    ListRefundsResp ListRefunds(1: ListRefundsReq req)
    RetryRefundResp RetryRefund(1: RetryRefundReq req)
    ListAbnormalResp ListAbnormal(1: ListAbnormalReq req)
    RefundAbnormalResp RefundAbnormal(1: RefundAbnormalReq req)
    // 履约
    CheckInResp CheckIn(1: CheckInReq req)
    ServiceProgressResp ServiceProgress(1: ServiceProgressReq req)
    // 未到店
    MarkNoShowResp MarkNoShow(1: MarkNoShowReq req)
    UpdateNoShowReasonResp UpdateNoShowReason(1: UpdateNoShowReasonReq req)
    // 报表与时钟
    DailyReportResp DailyReport(1: DailyReportReq req)
    GetClockResp GetClock()
    SetClockResp SetClock(1: SetClockReq req)
}
