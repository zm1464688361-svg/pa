package session

import (
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	tok := Token(7, time.Hour)
	uid, err := Parse(tok)
	if err != nil || uid != 7 {
		t.Fatalf("roundtrip: %v %d", err, uid)
	}
	// Cookie 安全字符：无空格与竖线
	for _, ch := range tok {
		if ch == ' ' || ch == '|' || ch == ';' || ch == ',' {
			t.Fatalf("token 含非 Cookie 安全字符: %q", tok)
		}
	}
}

func TestTokenInvalid(t *testing.T) {
	if _, err := Parse(Token(1, time.Hour) + "x"); err == nil {
		t.Fatal("篡改签名应拒绝")
	}
	if _, err := Parse(""); err == nil {
		t.Fatal("空 token 应拒绝")
	}
	if _, err := Parse(Token(1, -time.Hour)); err == nil {
		t.Fatal("过期 token 应拒绝")
	}
	if _, err := Parse("1.abc.def"); err == nil {
		t.Fatal("非法 exp 应拒绝")
	}
}

func TestCookieName(t *testing.T) {
	if CookieName() == "" {
		t.Fatal("cookie name 不能为空")
	}
}
