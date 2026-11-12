// 模拟支付渠道 pay_sim：独立进程。
// 支持触发：支付成功 / 失败 / 不回调（挂起）/ 重复成功通知 / 退款成功 / 退款失败。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/zm1464688361-svg/pa/internal/paysim"
	"github.com/zm1464688361-svg/pa/internal/sign"
)

type payment struct {
	NotifyID  string
	OrderNo   string
	TradeNo   string
	AmountFen int64
	Result    string // success / fail / none
	Seq       int
	Sent      bool
}

var (
	mu        sync.Mutex
	payments        = map[int64]*payment{}
	nextID    int64 = 1
	nextTrade int64 = 100000
)

func callbackURL() string {
	if v := os.Getenv("BIZ_CALLBACK"); v != "" {
		return v
	}
	if v := os.Getenv("GATEWAY_URL"); v != "" {
		return v
	}
	return "http://127.0.0.1:8081"
}

func main() {
	http.HandleFunc("GET /", adminPage)
	http.HandleFunc("POST /api/pay", apiPay)
	http.HandleFunc("POST /action/send/{id}/{result}", actionSend)
	http.HandleFunc("POST /action/dup/{id}", actionDup)
	http.HandleFunc("POST /action/refund", actionRefund)
	log.Println("pay_sim listening on :9002 (callback -> " + callbackURL() + ")")
	log.Fatal(http.ListenAndServe(":9002", nil))
}

// apiPay 网关调用：为订单创建支付通知。
// result=success/fail 立即回调；result=none 挂起等管理页手动触发。
func apiPay(w http.ResponseWriter, r *http.Request) {
	var t paysim.PayTrigger
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		jsonResp(w, 400, "bad json: "+err.Error())
		return
	}
	mu.Lock()
	id := nextID
	nextID++
	seq := 1
	p := &payment{
		NotifyID:  fmt.Sprintf("N%d-%d", id, seq),
		OrderNo:   t.OrderNo,
		TradeNo:   "T" + strconv.FormatInt(nextTrade, 10),
		Result:    t.Result,
		Seq:       seq,
		AmountFen: 9900,
	}
	nextTrade++
	payments[id] = p
	mu.Unlock()

	if t.Result == "success" || t.Result == "fail" {
		go sendPayNotify(p)
	}
	jsonResp(w, 200, "已受理，通知号 "+p.NotifyID)
}

// actionSend 管理页手动触发某笔挂起通知。
func actionSend(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	result := r.PathValue("result")
	mu.Lock()
	p := payments[id]
	if p == nil {
		mu.Unlock()
		jsonResp(w, 404, "支付记录不存在")
		return
	}
	p.Result = result
	p.Sent = false
	mu.Unlock()
	sendPayNotify(p)
	jsonResp(w, 200, "已发送 "+result+" 通知")
}

// actionDup 重复发送成功通知（不同通知号，验证幂等）。
func actionDup(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	mu.Lock()
	p := payments[id]
	if p == nil {
		mu.Unlock()
		jsonResp(w, 404, "支付记录不存在")
		return
	}
	p.Seq++
	dup := &payment{
		NotifyID: fmt.Sprintf("%s-dup%d", p.NotifyID, p.Seq),
		OrderNo:  p.OrderNo, TradeNo: p.TradeNo, AmountFen: p.AmountFen,
		Result: "success", Seq: p.Seq,
	}
	mu.Unlock()
	sendPayNotify(dup)
	jsonResp(w, 200, "已发送重复成功通知 "+dup.NotifyID)
}

// actionRefund 管理页手动触发退款结果回调（表单：refund_no + result）。
func actionRefund(w http.ResponseWriter, r *http.Request) {
	refundNo := r.FormValue("refund_no")
	result := r.FormValue("result")
	if refundNo == "" || result == "" {
		jsonResp(w, 400, "refund_no 与 result 必填")
		return
	}
	notifyID := fmt.Sprintf("RF-%d", time.Now().UnixNano())
	notify := paysim.RefundNotify{
		NotifyID: notifyID, RefundNo: refundNo, Result: result,
		Sign: sign.Refund(notifyID, refundNo, result),
	}
	go postCallback("/callback/refund", notify)
	jsonResp(w, 200, "退款回调已发送："+result)
}

func sendPayNotify(p *payment) {
	notify := paysim.PayNotify{
		NotifyID: p.NotifyID, OrderNo: p.OrderNo, TradeNo: p.TradeNo,
		AmountFen: p.AmountFen, Result: p.Result,
		Sign: sign.Pay(p.NotifyID, p.OrderNo, p.TradeNo, p.AmountFen, p.Result),
	}
	postCallback("/callback/pay", notify)
	mu.Lock()
	p.Sent = true
	mu.Unlock()
}

func postCallback(path string, payload interface{}) {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, callbackURL()+path, bytes.NewReader(b))
	if err != nil {
		log.Println("callback build:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("callback failed:", err)
		return
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&out)
	log.Printf("callback %s -> %v", path, out)
}

func jsonResp(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{"code": code, "msg": msg})
}

// adminPage 管理页：挂起/历史支付 + 手动退款回调表单。
func adminPage(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	ids := make([]int64, 0, len(payments))
	for id := range payments {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	slices.Reverse(ids)
	snapshot := make([]*payment, 0, len(ids))
	for _, id := range ids {
		p := *payments[id]
		snapshot = append(snapshot, &p)
	}
	mu.Unlock()

	fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>模拟支付渠道</title>
<style>body{font-family:system-ui;margin:24px;background:#f6f7f5}h1{color:#0b3d2e}
table{border-collapse:collapse;background:#fff;width:100%}
th,td{border:1px solid #d8ddd8;padding:6px 10px;font-size:14px}
button{background:#0b3d2e;color:#fff;border:0;padding:4px 10px;margin:2px;border-radius:4px;cursor:pointer}
button.gray{background:#888}.box{background:#fff;padding:16px;border-radius:8px;margin-bottom:20px}
input{padding:6px;margin-right:8px}</style>
<h1>模拟支付渠道（教学演示）</h1>
<div class="box"><h3>手动退款回调</h3>
<form method="post" action="/action/refund">
<input name="refund_no" placeholder="退款单号 如 R20261112...（在客服退款页查看）">
<select name="result"><option value="success">退款成功</option><option value="fail">退款失败</option></select>
<button type="submit">发送回调</button></form>
<p style="color:#888">退款单号在「客服 / 退款跟进」页查看；重复发送同一成功回调由业务侧幂等处理。</p></div>
<table><tr><th>ID</th><th>通知号</th><th>订单号</th><th>交易号</th><th>金额(分)</th><th>结果</th><th>已回调</th><th>操作</th></tr>`)
	for _, p := range snapshot {
		fmt.Fprintf(w, `<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td><td>%v</td>
<td>
<form method="post" action="/action/send/%d/success" style="display:inline"><button>成功</button></form>
<form method="post" action="/action/send/%d/fail" style="display:inline"><button class="gray">失败</button></form>
<form method="post" action="/action/dup/%d" style="display:inline"><button class="gray">重复成功</button></form>
</td></tr>`, p.Seq, p.NotifyID, p.OrderNo, p.TradeNo, p.AmountFen, p.Result, p.Sent, p.Seq, p.Seq, p.Seq)
	}
	fmt.Fprint(w, `</table><p style="color:#888">挂起（不回调）场景：网关触发时选择"挂起"，再回到本页手动决定结果。</p>`)
}

var _ = context.Background
