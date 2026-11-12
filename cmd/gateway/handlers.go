package main

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/paysim"
	"github.com/zm1464688361-svg/pa/internal/session"
	petshop "github.com/zm1464688361-svg/pa/kitex_gen/petshop"
)

type actorT = petshop.Actor

const sessionTTL = 24 * time.Hour

// ---------- 登录 ----------

var presetAccounts = []map[string]string{
	{"username": "owner", "role": "店长"},
	{"username": "user1", "role": "宠物主人"},
	{"username": "user2", "role": "宠物主人"},
	{"username": "user3", "role": "宠物主人"},
	{"username": "front", "role": "前台/服务人员"},
	{"username": "support", "role": "客服"},
}

func hLogin(ctx context.Context, c *app.RequestContext) {
	render(c, "login.html", map[string]interface{}{"Accounts": presetAccounts})
}

func hLoginPost(ctx context.Context, c *app.RequestContext) {
	username := string(c.PostForm("username"))
	var u model.User
	if err := db.Where("username = ?", username).First(&u).Error; err != nil {
		render(c, "login.html", map[string]interface{}{
			"Accounts": presetAccounts, "Error": "账号不存在（仅支持预置账号）",
		})
		return
	}
	c.SetCookie(session.CookieName(), session.Token(u.ID, sessionTTL), int(sessionTTL.Seconds()), "/", "", protocol.CookieSameSiteLaxMode, false, true)
	c.Redirect(302, []byte(homeOf(&u)))
}

func hLogout(ctx context.Context, c *app.RequestContext) {
	c.SetCookie(session.CookieName(), "", -1, "/", "", protocol.CookieSameSiteLaxMode, false, true)
	c.Redirect(302, []byte("/login"))
}

func hHome(ctx context.Context, c *app.RequestContext) {
	u := currentUser(c)
	if u == nil {
		c.Redirect(302, []byte("/login"))
		return
	}
	c.Redirect(302, []byte(homeOf(u)))
}

// ---------- 宠物主人：时段与预约 ----------

func hSlots(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	resp, err := svc.ListSlots(ctx, &petshop.ListSlotsReq{Actor: actor(u)})
	if err != nil {
		render(c, "error.html", map[string]interface{}{"Error": err.Error()})
		return
	}
	// 我的宠物
	var pets []model.Pet
	db.Where("user_id = ?", u.ID).Find(&pets)
	render(c, "slots.html", map[string]interface{}{
		"Slots": resp.Slots, "ItemName": resp.ItemName, "PriceFen": resp.PriceFen,
		"Pets": pets, "Base": resp.Base,
	})
}

func hCreateBooking(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	slotID := atoi64(string(c.PostForm("slot_id")))
	petID := atoi64(string(c.PostForm("pet_id")))
	resp, err := svc.CreateBooking(ctx, &petshop.CreateBookingReq{Actor: actor(u), SlotId: slotID, PetId: petID})
	if err != nil || resp.Base.Code != 0 {
		msg := resp.Base.Msg
		if err != nil {
			msg = err.Error()
		}
		render(c, "slots_result.html", map[string]interface{}{"Ok": false, "Msg": msg, "BackTo": "/slots"})
		return
	}
	c.Redirect(302, []byte("/bookings?new="+strconv.FormatInt(resp.Booking.Id, 10)))
}

func hMyBookings(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	resp, err := svc.GetBookings(ctx, &petshop.GetBookingsReq{Actor: actor(u)})
	if err != nil {
		render(c, "error.html", map[string]interface{}{"Error": err.Error()})
		return
	}
	var pets []model.Pet
	db.Where("user_id = ?", u.ID).Find(&pets)
	render(c, "bookings.html", map[string]interface{}{
		"Bookings": resp.Bookings, "Pets": pets,
		"NewID": c.Query("new"), "Base": resp.Base,
	})
}

func hBookingDetail(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	id := atoi64(string(c.Param("id")))
	resp, err := svc.GetBookingDetail(ctx, &petshop.GetBookingDetailReq{Actor: actor(u), BookingId: id})
	if err != nil || resp.Base.Code != 0 {
		render(c, "error.html", map[string]interface{}{
			"Error": msgOf(err, resp.GetBase().Msg),
		})
		return
	}
	render(c, "booking_detail.html", map[string]interface{}{
		"B": resp.Booking, "Base": resp.Base,
	})
}

func hCancelUnpaid(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	id := atoi64(string(c.Param("id")))
	resp, _ := svc.CancelUnpaid(ctx, &petshop.CancelUnpaidReq{Actor: actor(u), BookingId: id})
	c.Redirect(302, []byte("/bookings?msg="+urlEscape(resp.GetMsg())))
}

func hCancelForm(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	id := atoi64(string(c.Param("id")))
	resp, err := svc.GetBookingDetail(ctx, &petshop.GetBookingDetailReq{Actor: actor(u), BookingId: id})
	if err != nil || resp.Base.Code != 0 {
		render(c, "error.html", map[string]interface{}{"Error": msgOf(err, resp.GetBase().Msg)})
		return
	}
	render(c, "cancel_form.html", map[string]interface{}{"B": resp.Booking})
}

func hSubmitCancel(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	id := atoi64(string(c.Param("id")))
	reason := string(c.PostForm("reason"))
	resp, _ := svc.SubmitCancel(ctx, &petshop.SubmitCancelReq{Actor: actor(u), BookingId: id, Reason: reason})
	render(c, "slots_result.html", map[string]interface{}{
		"Ok": resp.Base.Code == 0, "Msg": resp.Msg, "Detail": resp.Base.Msg, "BackTo": "/bookings",
	})
}

// hPayHold 挂起支付（不回调），用于演示"关单后迟到支付"场景。
func hPayHold(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	orderNo := string(c.Param("orderNo"))
	_, err := paysim.TriggerPay(ctx, paysim.PayTrigger{OrderNo: orderNo, Result: "none"})
	render(c, "slots_result.html", map[string]interface{}{
		"Ok":     err == nil,
		"Msg":    "支付已挂起（渠道不回调）",
		"Detail": "可稍后到模拟渠道管理页（127.0.0.1:9002）手动发送成功/失败通知，或先用教学时钟把时间拨过 5 分钟支付期，再发送成功通知演示'关单后迟到支付→异常支付'。",
		"BackTo": "/bookings",
	})
}

// hTriggerPay 主人点击"模拟支付"：调用 pay_sim 触发支付通知（异步回调业务）。
func hTriggerPay(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	orderNo := string(c.Param("orderNo"))
	result := string(c.PostForm("result")) // success / fail / none
	if result == "" {
		result = "success"
	}
	resp, err := paysim.TriggerPay(ctx, paysim.PayTrigger{
		OrderNo: orderNo, Result: result,
	})
	msg := "已提交模拟支付：" + result
	if err != nil || resp == nil {
		msg = "模拟支付提交失败：" + fmt.Sprint(err)
	}
	render(c, "slots_result.html", map[string]interface{}{
		"Ok": err == nil, "Msg": msg,
		"Detail": "支付结果经模拟渠道异步回调确认，请刷新订单页查看最新状态",
		"BackTo": "/bookings",
	})
}

// ---------- pay_sim 回调 ----------

func hPayCallback(ctx context.Context, c *app.RequestContext) {
	var body paysim.PayNotify
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, map[string]string{"msg": "bad json"})
		return
	}
	resp, err := svc.PayNotify(ctx, &petshop.PayNotifyReq{
		NotifyId: body.NotifyID, OrderNo: body.OrderNo, ChannelTradeNo: body.TradeNo,
		AmountFen: body.AmountFen, PayResult_: body.Result, Sign: body.Sign,
	})
	code := int32(200)
	if err != nil || resp.Base.Code != 0 {
		code = 400
	}
	c.JSON(int(code), map[string]interface{}{"code": resp.GetBase().Code, "msg": resp.GetMsg()})
}

func hRefundCallback(ctx context.Context, c *app.RequestContext) {
	var body paysim.RefundNotify
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, map[string]string{"msg": "bad json"})
		return
	}
	resp, err := svc.RefundNotify(ctx, &petshop.RefundNotifyReq{
		NotifyId: body.NotifyID, RefundNo: body.RefundNo, PayResult_: body.Result, Sign: body.Sign,
	})
	code := 200
	if err != nil || resp.Base.Code != 0 {
		code = 400
	}
	c.JSON(code, map[string]interface{}{"code": resp.GetBase().Code, "msg": resp.GetMsg()})
}

// ---------- 前台 ----------

func hFront(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	resp, err := svc.GetBookings(ctx, &petshop.GetBookingsReq{Actor: actor(u)})
	if err != nil {
		render(c, "error.html", map[string]interface{}{"Error": err.Error()})
		return
	}
	render(c, "front.html", map[string]interface{}{
		"Bookings": resp.Bookings, "Msg": c.Query("msg"), "Base": resp.Base,
	})
}

func hCheckIn(ctx context.Context, c *app.RequestContext) {
	frontAction(ctx, c, func(id int64) (string, int32) {
		resp, _ := svc.CheckIn(ctx, &petshop.CheckInReq{Actor: actor(requireUser(c)), BookingId: id})
		return resp.Msg, resp.Base.Code
	}, "/front")
}

func hStart(ctx context.Context, c *app.RequestContext) {
	frontAction(ctx, c, func(id int64) (string, int32) {
		resp, _ := svc.ServiceProgress(ctx, &petshop.ServiceProgressReq{Actor: actor(requireUser(c)), BookingId: id, Action: "start"})
		return resp.Msg, resp.Base.Code
	}, "/front")
}

func hComplete(ctx context.Context, c *app.RequestContext) {
	frontAction(ctx, c, func(id int64) (string, int32) {
		resp, _ := svc.ServiceProgress(ctx, &petshop.ServiceProgressReq{Actor: actor(requireUser(c)), BookingId: id, Action: "complete"})
		return resp.Msg, resp.Base.Code
	}, "/front")
}

func frontAction(ctx context.Context, c *app.RequestContext, fn func(int64) (string, int32), back string) {
	u := requireUser(c)
	if u == nil {
		return
	}
	id := atoi64(string(c.Param("id")))
	msg, code := fn(id)
	if code != 0 {
		msg = "[" + strconv.Itoa(int(code)) + "] " + msg
	}
	c.Redirect(302, []byte(back+"?msg="+urlEscape(msg)))
}

// ---------- 客服 ----------

func hSupport(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	resp, err := svc.ListCancelRequests(ctx, &petshop.ListCancelRequestsReq{Actor: actor(u)})
	if e := firstErr(err, resp); e != nil {
		render(c, "error.html", map[string]interface{}{"Error": e.Error()})
		return
	}
	render(c, "support.html", map[string]interface{}{
		"Requests": resp.Requests, "Msg": c.Query("msg"), "Base": resp.Base,
	})
}

func hReviewCancel(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	reqID := atoi64(string(c.PostForm("request_id")))
	approve := string(c.PostForm("decision")) == "approve"
	reason := string(c.PostForm("reason"))
	resp, _ := svc.ReviewCancel(ctx, &petshop.ReviewCancelReq{
		Actor: actor(u), RequestId: reqID, Approve: approve, Reason: reason,
	})
	c.Redirect(302, []byte("/support?msg="+urlEscape(resp.Msg)))
}

func hSupportRefunds(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	resp, err := svc.ListRefunds(ctx, &petshop.ListRefundsReq{Actor: actor(u)})
	if e := firstErr(err, resp); e != nil {
		render(c, "error.html", map[string]interface{}{"Error": e.Error()})
		return
	}
	render(c, "refunds.html", map[string]interface{}{
		"Refunds": resp.Refunds, "Msg": c.Query("msg"), "Base": resp.Base,
	})
}

func hRetryRefund(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	id := atoi64(string(c.Param("id")))
	resp, _ := svc.RetryRefund(ctx, &petshop.RetryRefundReq{Actor: actor(u), RefundId: id})
	c.Redirect(302, []byte("/support/refunds?msg="+urlEscape(resp.Msg)))
}

func hSupportAbnormal(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	resp, err := svc.ListAbnormal(ctx, &petshop.ListAbnormalReq{Actor: actor(u)})
	if e := firstErr(err, resp); e != nil {
		render(c, "error.html", map[string]interface{}{"Error": e.Error()})
		return
	}
	render(c, "abnormal.html", map[string]interface{}{
		"Items": resp.Items, "Msg": c.Query("msg"), "Base": resp.Base,
	})
}

func hRefundAbnormal(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	id := atoi64(string(c.Param("id")))
	resp, _ := svc.RefundAbnormal(ctx, &petshop.RefundAbnormalReq{Actor: actor(u), AbnormalId: id})
	c.Redirect(302, []byte("/support/abnormal?msg="+urlEscape(resp.Msg)))
}

// ---------- 店长 ----------

func hOwner(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	date := c.Query("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	rep, err := svc.DailyReport(ctx, &petshop.DailyReportReq{Actor: actor(u), Date: date})
	if e := firstErr(err, rep); e != nil {
		render(c, "error.html", map[string]interface{}{"Error": e.Error()})
		return
	}
	bookings, _ := svc.GetBookings(ctx, &petshop.GetBookingsReq{Actor: actor(u)})
	render(c, "owner.html", map[string]interface{}{
		"Report": rep, "Date": date, "Bookings": bookings.Bookings,
		"Msg": c.Query("msg"), "Base": rep.Base,
	})
}

func hCreateSlot(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	resp, _ := svc.CreateSlot(ctx, &petshop.CreateSlotReq{
		Actor:     actor(u),
		StartTime: string(c.PostForm("start_time")),
		EndTime:   string(c.PostForm("end_time")),
		Capacity:  int32(atoi64(string(c.PostForm("capacity")))),
	})
	c.Redirect(302, []byte("/owner?msg="+urlEscape(resp.Base.Msg)))
}

func hMarkNoShow(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	id := atoi64(string(c.PostForm("booking_id")))
	reason := string(c.PostForm("reason"))
	resp, _ := svc.MarkNoShow(ctx, &petshop.MarkNoShowReq{Actor: actor(u), BookingId: id, Reason: reason})
	c.Redirect(302, []byte("/owner?msg="+urlEscape(resp.Msg)))
}

func hNoShowReason(ctx context.Context, c *app.RequestContext) {
	u := requireUser(c)
	if u == nil {
		return
	}
	id := atoi64(string(c.Param("id")))
	resp, _ := svc.UpdateNoShowReason(ctx, &petshop.UpdateNoShowReasonReq{
		Actor: actor(u), BookingId: id, Reason: string(c.PostForm("reason")),
	})
	c.Redirect(302, []byte("/owner?msg="+urlEscape(resp.Msg)))
}

// ---------- 教学时钟 ----------

func hClock(ctx context.Context, c *app.RequestContext) {
	resp, err := svc.GetClock(ctx)
	if err != nil {
		render(c, "error.html", map[string]interface{}{"Error": err.Error()})
		return
	}
	render(c, "clock.html", map[string]interface{}{
		"Now": resp.Now, "OffsetSec": resp.OffsetSec,
	})
}

func hClockSet(ctx context.Context, c *app.RequestContext) {
	offset := atoi64(string(c.PostForm("offset_sec")))
	svc.SetClock(ctx, &petshop.SetClockReq{OffsetSec: offset})
	c.Redirect(302, []byte("/clock"))
}

// ---------- 工具 ----------

func msgOf(err error, def string) string {
	if err != nil {
		return err.Error()
	}
	return def
}

// firstErr 合并 RPC 传输错误与业务错误（Base.Code != 0），供只读页面统一拦截。
func firstErr(err error, resp interface{ GetBase() *petshop.BaseResp }) error {
	if err != nil {
		return err
	}
	if resp == nil {
		return nil
	}
	if b := resp.GetBase(); b != nil && b.Code != 0 {
		return fmt.Errorf("[%d] %s", b.Code, b.Msg)
	}
	return nil
}

func urlEscape(s string) string {
	return url.QueryEscape(s)
}
