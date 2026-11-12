// Package sign 业务服务与模拟支付渠道共享的回调签名（教学演示密钥）。
package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const Secret = "petshop-demo-secret"

// Pay 支付回调签名：HMAC-SHA256(notifyId|orderNo|tradeNo|amount|result)。
func Pay(notifyID, orderNo, tradeNo string, amountFen int64, result string) string {
	return mac(fmt.Sprintf("%s|%s|%s|%d|%s", notifyID, orderNo, tradeNo, amountFen, result))
}

// Refund 退款回调签名：HMAC-SHA256(notifyId|refundNo|result)。
func Refund(notifyID, refundNo, result string) string {
	return mac(fmt.Sprintf("%s|%s|%s", notifyID, refundNo, result))
}

func mac(raw string) string {
	m := hmac.New(sha256.New, []byte(Secret))
	m.Write([]byte(raw))
	return hex.EncodeToString(m.Sum(nil))
}

func Equal(a, b string) bool { return hmac.Equal([]byte(a), []byte(b)) }
