package sign

import "testing"

func TestPaySignDeterministic(t *testing.T) {
	a := Pay("N1", "B1", "T1", 9900, "success")
	b := Pay("N1", "B1", "T1", 9900, "success")
	if a != b || len(a) != 64 {
		t.Fatalf("签名应确定且为 hex: %s", a)
	}
	if Pay("N1", "B1", "T1", 9901, "success") == a {
		t.Fatal("金额不同签名应不同")
	}
	if !Equal(a, a) || Equal(a, "") {
		t.Fatal("Equal 行为错误")
	}
}

func TestRefundSign(t *testing.T) {
	a := Refund("N1", "R1", "success")
	if a != Refund("N1", "R1", "success") || a == Refund("N1", "R1", "fail") {
		t.Fatal("退款签名行为错误")
	}
}
