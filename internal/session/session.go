// Package session 预置账号登录会话（HMAC 签名 Cookie）。
package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const secret = "petshop-session-secret"
const cookieName = "petshop_sess"

// Token 会话令牌：userID.exp.签名（分隔符使用 Cookie 安全字符 "."）。
func Token(userID int64, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	payload := strconv.FormatInt(userID, 10) + "." + strconv.FormatInt(exp, 10)
	return payload + "." + sign(payload)
}

// Parse 校验并解析会话令牌，返回 userID。
func Parse(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("bad token")
	}
	payload := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(sign(payload)), []byte(parts[2])) {
		return 0, fmt.Errorf("bad signature")
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return 0, fmt.Errorf("expired")
	}
	uid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	return uid, nil
}

func sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// CookieName 供 HTTP 层读写。
func CookieName() string { return cookieName }
