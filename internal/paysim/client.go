// Package paysim 与模拟支付渠道（cmd/paysim）交互的客户端与消息结构。
package paysim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// PayTrigger 网关触发模拟支付的请求。
type PayTrigger struct {
	OrderNo string `json:"order_no"`
	Result  string `json:"result"` // success / fail / none(挂起待手动)
}

// PayNotify 渠道 → 业务服务的支付回调。
type PayNotify struct {
	NotifyID  string `json:"notify_id"`
	OrderNo   string `json:"order_no"`
	TradeNo   string `json:"channel_trade_no"`
	AmountFen int64  `json:"amount_fen"`
	Result    string `json:"pay_result"`
	Sign      string `json:"sign"`
}

// RefundNotify 渠道 → 业务服务的退款回调。
type RefundNotify struct {
	NotifyID string `json:"notify_id"`
	RefundNo string `json:"refund_no"`
	Result   string `json:"pay_result"`
	Sign     string `json:"sign"`
}

type baseResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func baseURL() string {
	if v := os.Getenv("PAY_SIM_URL"); v != "" {
		return v
	}
	return "http://127.0.0.1:9002"
}

func post(ctx context.Context, path string, payload interface{}) (*baseResp, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL()+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := &baseResp{}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("decode paysim resp: %w", err)
	}
	return out, nil
}

// TriggerPay 让模拟渠道对指定订单发起支付通知（result=none 时挂起，等管理页手动触发）。
func TriggerPay(ctx context.Context, t PayTrigger) (*baseResp, error) {
	return post(ctx, "/api/pay", t)
}
