package main

import (
	"context"
	"log"

	"github.com/zm1464688361-svg/pa/internal/biz"
	"github.com/zm1464688361-svg/pa/internal/errs"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/kitex_gen/petshop"
)

// AppointmentServiceImpl Kitex RPC 实现，委托业务层。
type AppointmentServiceImpl struct {
	B *biz.Biz
}

func base(err error) *petshop.BaseResp {
	if err == nil {
		return &petshop.BaseResp{Code: 0, Msg: "ok"}
	}
	code, msg := int32(5000), err.Error()
	if be, ok := err.(*errs.BizError); ok {
		code, msg = int32(be.Code), be.Msg
	}
	return &petshop.BaseResp{Code: code, Msg: msg}
}

// ---------- 时段 ----------

func (s *AppointmentServiceImpl) CreateSlot(ctx context.Context, req *petshop.CreateSlotReq) (resp *petshop.CreateSlotResp, err error) {
	slot, e := s.B.CreateSlot(ctx, req.Actor.UserId, req.Actor.Role, req.StartTime, req.EndTime, req.Capacity)
	resp = &petshop.CreateSlotResp{Base: base(e)}
	if e == nil {
		resp.Slot = toSlot(slot)
	}
	return resp, nil
}

func (s *AppointmentServiceImpl) ListSlots(ctx context.Context, req *petshop.ListSlotsReq) (resp *petshop.ListSlotsResp, err error) {
	views, item, e := s.B.ListSlots(ctx, req.Actor.Role)
	resp = &petshop.ListSlotsResp{Base: base(e)}
	if e == nil {
		resp.ItemName, resp.PriceFen = item.Name, item.PriceFen
		for i := range views {
			resp.Slots = append(resp.Slots, &petshop.SlotView{
				Slot: toSlot(views[i].Slot), Bookable: views[i].Bookable, Reason: views[i].Reason,
			})
		}
	}
	return resp, nil
}

func toSlot(s *model.TimeSlot) *petshop.TimeSlot {
	if s == nil {
		return nil
	}
	return &petshop.TimeSlot{
		Id: s.ID, StartTime: s.StartTime.Format("2006-01-02 15:04"),
		EndTime: s.EndTime.Format("2006-01-02 15:04"),
		Capacity: s.Capacity, Remaining: s.Remaining, Status: s.Status,
	}
}

// ---------- 预约 ----------

func (s *AppointmentServiceImpl) CreateBooking(ctx context.Context, req *petshop.CreateBookingReq) (resp *petshop.CreateBookingResp, err error) {
	actor := toUser(req.Actor)
	ap, e := s.B.CreateBooking(ctx, actor, req.SlotId, req.PetId)
	resp = &petshop.CreateBookingResp{Base: base(e)}
	if e == nil {
		resp.Booking = toBooking(ap, "", "")
	}
	return resp, nil
}

func (s *AppointmentServiceImpl) GetBookings(ctx context.Context, req *petshop.GetBookingsReq) (resp *petshop.GetBookingsResp, err error) {
	rows, e := s.B.ListBookings(ctx, toUser(req.Actor), req.SlotId)
	resp = &petshop.GetBookingsResp{Base: base(e)}
	if e == nil {
		for i := range rows {
			resp.Bookings = append(resp.Bookings, toBookingFull(&rows[i]))
		}
	}
	return resp, nil
}

func (s *AppointmentServiceImpl) GetBookingDetail(ctx context.Context, req *petshop.GetBookingDetailReq) (resp *petshop.GetBookingDetailResp, err error) {
	ap, status, e := s.B.GetBookingDetail(ctx, toUser(req.Actor), req.BookingId)
	resp = &petshop.GetBookingDetailResp{Base: base(e)}
	if e == nil {
		resp.Booking = toBooking(ap, status, "")
	}
	return resp, nil
}

func (s *AppointmentServiceImpl) CancelUnpaid(ctx context.Context, req *petshop.CancelUnpaidReq) (resp *petshop.CancelUnpaidResp, err error) {
	msg, e := s.B.CancelUnpaid(ctx, toUser(req.Actor), req.BookingId)
	return &petshop.CancelUnpaidResp{Base: base(e), Msg: msg}, nil
}

// ---------- 取消申请 ----------

func (s *AppointmentServiceImpl) SubmitCancel(ctx context.Context, req *petshop.SubmitCancelReq) (resp *petshop.SubmitCancelResp, err error) {
	cr, msg, e := s.B.SubmitCancel(ctx, toUser(req.Actor), req.BookingId, req.Reason)
	resp = &petshop.SubmitCancelResp{Base: base(e), Msg: msg}
	if e == nil && cr != nil {
		resp.Request = toCancelRequest(cr)
	}
	return resp, nil
}

func (s *AppointmentServiceImpl) ListCancelRequests(ctx context.Context, req *petshop.ListCancelRequestsReq) (resp *petshop.ListCancelRequestsResp, err error) {
	views, e := s.B.ListCancelRequests(ctx, toUser(req.Actor), req.Status)
	resp = &petshop.ListCancelRequestsResp{Base: base(e)}
	if e == nil {
		for i := range views {
			v := &views[i]
			resp.Requests = append(resp.Requests, &petshop.CancelRequestView{
				Request: toCancelRequest(v.Request), OrderNo: v.OrderNo, UserPet: v.UserPet,
				SlotRange: v.SlotRange, StatusText: v.StatusText, AmountFen: v.AmountFen,
			})
		}
	}
	return resp, nil
}

func (s *AppointmentServiceImpl) ReviewCancel(ctx context.Context, req *petshop.ReviewCancelReq) (resp *petshop.ReviewCancelResp, err error) {
	msg, e := s.B.ReviewCancel(ctx, toUser(req.Actor), req.RequestId, req.Approve, req.Reason)
	return &petshop.ReviewCancelResp{Base: base(e), Msg: msg}, nil
}

func toCancelRequest(cr *model.CancelRequest) *petshop.CancelRequest {
	if cr == nil {
		return nil
	}
	return &petshop.CancelRequest{
		Id: cr.ID, BookingId: cr.BookingID, Reason: cr.Reason, Status: cr.Status,
		ReviewReason: cr.ReviewReason, Urgent: cr.Urgent,
		CreatedAt: cr.CreatedAt.Format("01-02 15:04"),
	}
}

// ---------- 支付/退款回调 ----------

func (s *AppointmentServiceImpl) PayNotify(ctx context.Context, req *petshop.PayNotifyReq) (resp *petshop.PayNotifyResp, err error) {
	msg, e := s.B.PayNotify(ctx, req.NotifyId, req.OrderNo, req.ChannelTradeNo, req.AmountFen, req.PayResult_, req.Sign)
	return &petshop.PayNotifyResp{Base: base(e), Msg: msg}, nil
}

func (s *AppointmentServiceImpl) RefundNotify(ctx context.Context, req *petshop.RefundNotifyReq) (resp *petshop.RefundNotifyResp, err error) {
	msg, e := s.B.RefundNotify(ctx, req.NotifyId, req.RefundNo, req.PayResult_, req.Sign)
	return &petshop.RefundNotifyResp{Base: base(e), Msg: msg}, nil
}

// ---------- 退款与异常 ----------

func (s *AppointmentServiceImpl) ListRefunds(ctx context.Context, req *petshop.ListRefundsReq) (resp *petshop.ListRefundsResp, err error) {
	views, e := s.B.ListRefunds(ctx, toUser(req.Actor), req.Status)
	resp = &petshop.ListRefundsResp{Base: base(e)}
	if e == nil {
		for i := range views {
			resp.Refunds = append(resp.Refunds, toRefund(&views[i]))
		}
	}
	return resp, nil
}

func toRefund(v *biz.RefundView) *petshop.Refund {
	r := v.Refund
	return &petshop.Refund{
		Id: r.ID, RefundNo: r.RefundNo, BookingId: r.BookingID, OrderNo: r.OrderNo,
		AmountFen: r.AmountFen, Status: r.Status, RetryCount: r.RetryCount,
		FailReason: r.FailReason, Kind: r.Kind, StatusText: v.StatusText,
		TimeoutFlag: v.TimeoutFlag, CreatedAt: r.CreatedAt.Format("01-02 15:04"),
	}
}

func (s *AppointmentServiceImpl) RetryRefund(ctx context.Context, req *petshop.RetryRefundReq) (resp *petshop.RetryRefundResp, err error) {
	msg, e := s.B.RetryRefund(ctx, toUser(req.Actor), req.RefundId)
	return &petshop.RetryRefundResp{Base: base(e), Msg: msg}, nil
}

func (s *AppointmentServiceImpl) ListAbnormal(ctx context.Context, req *petshop.ListAbnormalReq) (resp *petshop.ListAbnormalResp, err error) {
	views, e := s.B.ListAbnormal(ctx, toUser(req.Actor))
	resp = &petshop.ListAbnormalResp{Base: base(e)}
	if e == nil {
		for i := range views {
			v := &views[i]
			resp.Items = append(resp.Items, &petshop.AbnormalPayment{
				Id: v.Item.ID, OrderNo: v.Item.OrderNo, ChannelTradeNo: v.Item.ChannelTradeNo,
				AmountFen: v.Item.AmountFen, Reason: v.Item.Reason, Status: v.Item.Status,
				StatusText: v.StatusText, CreatedAt: v.Item.CreatedAt.Format("01-02 15:04"),
			})
		}
	}
	return resp, nil
}

func (s *AppointmentServiceImpl) RefundAbnormal(ctx context.Context, req *petshop.RefundAbnormalReq) (resp *petshop.RefundAbnormalResp, err error) {
	msg, e := s.B.RefundAbnormal(ctx, toUser(req.Actor), req.AbnormalId)
	return &petshop.RefundAbnormalResp{Base: base(e), Msg: msg}, nil
}

// ---------- 履约 ----------

func (s *AppointmentServiceImpl) CheckIn(ctx context.Context, req *petshop.CheckInReq) (resp *petshop.CheckInResp, err error) {
	msg, e := s.B.CheckIn(ctx, toUser(req.Actor), req.BookingId)
	return &petshop.CheckInResp{Base: base(e), Msg: msg}, nil
}

func (s *AppointmentServiceImpl) ServiceProgress(ctx context.Context, req *petshop.ServiceProgressReq) (resp *petshop.ServiceProgressResp, err error) {
	msg, e := s.B.ServiceProgress(ctx, toUser(req.Actor), req.BookingId, req.Action)
	return &petshop.ServiceProgressResp{Base: base(e), Msg: msg}, nil
}

// ---------- 未到店 ----------

func (s *AppointmentServiceImpl) MarkNoShow(ctx context.Context, req *petshop.MarkNoShowReq) (resp *petshop.MarkNoShowResp, err error) {
	msg, e := s.B.MarkNoShow(ctx, toUser(req.Actor), req.BookingId, req.Reason)
	return &petshop.MarkNoShowResp{Base: base(e), Msg: msg}, nil
}

func (s *AppointmentServiceImpl) UpdateNoShowReason(ctx context.Context, req *petshop.UpdateNoShowReasonReq) (resp *petshop.UpdateNoShowReasonResp, err error) {
	msg, e := s.B.UpdateNoShowReason(ctx, toUser(req.Actor), req.BookingId, req.Reason)
	return &petshop.UpdateNoShowReasonResp{Base: base(e), Msg: msg}, nil
}

// ---------- 报表与时钟 ----------

func (s *AppointmentServiceImpl) DailyReport(ctx context.Context, req *petshop.DailyReportReq) (resp *petshop.DailyReportResp, err error) {
	rep, e := s.B.DailyReport(ctx, toUser(req.Actor), req.Date)
	resp = &petshop.DailyReportResp{Base: base(e)}
	if e == nil {
		resp.ReceivedFen, resp.RefundedFen, resp.NetFen = rep.ReceivedFen, rep.RefundedFen, rep.NetFen
		resp.RefundPendingFen = rep.RefundFollowFen
		for i := range rep.SlotRows {
			r := &rep.SlotRows[i]
			resp.SlotRows = append(resp.SlotRows, &petshop.SlotRow{
				SlotRange: r.SlotRange, ValidCount: int32(r.Valid), CompletedCount: int32(r.Completed),
				CancelledCount: int32(r.Cancelled), NoShowCount: int32(r.NoShow),
				RefundPendingCount: int32(r.RefundFollow),
			})
		}
		for i := range rep.Records {
			r := &rep.Records[i]
			resp.Records = append(resp.Records, &petshop.RecordRow{
				Time: r.CreatedAt, OrderNo: r.OrderNo, UserPet: r.UserPet,
				SlotRange: r.SlotRange, StatusText: r.StatusText, Detail: r.Detail, AmountFen: r.AmountFen,
			})
		}
	}
	return resp, nil
}

func (s *AppointmentServiceImpl) GetClock(ctx context.Context) (resp *petshop.GetClockResp, err error) {
	return &petshop.GetClockResp{
		Base: &petshop.BaseResp{Code: 0, Msg: "ok"},
		Now:  s.B.Clock.Now().Format("2006-01-02 15:04:05"),
		OffsetSec: s.B.Clock.Offset(),
	}, nil
}

func (s *AppointmentServiceImpl) SetClock(ctx context.Context, req *petshop.SetClockReq) (resp *petshop.SetClockResp, err error) {
	e := s.B.Clock.SetOffset(req.OffsetSec)
	return &petshop.SetClockResp{Base: base(e)}, nil
}

// ---------- 转换辅助 ----------

func toUser(a *petshop.Actor) *model.User {
	return &model.User{ID: a.UserId, Role: a.Role, ShopID: a.ShopId}
}

func toBooking(ap *model.Appointment, statusText, petName string) *petshop.Booking {
	return &petshop.Booking{
		Id: ap.ID, OrderNo: ap.OrderNo, UserId: ap.UserID, PetId: ap.PetID,
		Status: ap.Status, AmountFen: ap.AmountFen,
		PayDeadline: ap.PayDeadline.Format("15:04:05"),
		StatusText:  statusText, PetName: petName,
		CreatedAt: ap.CreatedAt.Format("01-02 15:04"),
	}
}

func toBookingFull(r *biz.BookingRow) *petshop.Booking {
	bk := toBooking(r.Ap, r.StatusText, r.PetName)
	bk.UserName = r.UserName
	bk.SlotStart = r.SlotStart.Format("2006-01-02 15:04")
	bk.SlotEnd = r.SlotEnd.Format("15:04")
	return bk
}

var _ = log.Println
