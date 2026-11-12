package paysim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestTriggerPayAndPost(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(200)
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()
	t.Setenv("PAY_SIM_URL", srv.URL)
	os.Unsetenv("PAY_SIM_URL")
	os.Setenv("PAY_SIM_URL", srv.URL)

	resp, err := TriggerPay(context.Background(), PayTrigger{OrderNo: "B1", Result: "success"})
	if err != nil || resp.Code != 0 || resp.Msg != "ok" {
		t.Fatalf("TriggerPay: %v %+v", err, resp)
	}
	if gotPath != "/api/pay" || gotBody == "" {
		t.Fatalf("请求路径/体: %s %s", gotPath, gotBody)
	}
	os.Unsetenv("PAY_SIM_URL")
}

func TestBaseURLDefault(t *testing.T) {
	os.Unsetenv("PAY_SIM_URL")
	if baseURL() == "" {
		t.Fatal("默认地址不能为空")
	}
	os.Setenv("PAY_SIM_URL", "http://x")
	if baseURL() != "http://x" {
		t.Fatal("环境变量优先")
	}
	os.Unsetenv("PAY_SIM_URL")
}

func TestPostBadServer(t *testing.T) {
	os.Setenv("PAY_SIM_URL", "http://127.0.0.1:1")
	defer os.Unsetenv("PAY_SIM_URL")
	if _, err := TriggerPay(context.Background(), PayTrigger{OrderNo: "B"}); err == nil {
		t.Fatal("不可达服务应返回错误")
	}
}
