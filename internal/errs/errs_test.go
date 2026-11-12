package errs

import "testing"

func TestBizErrorFormat(t *testing.T) {
	e := New(1234, "测试错误")
	if e.Error() != "[1234] 测试错误" {
		t.Fatalf("格式: %s", e.Error())
	}
	w := e.With("参数 %d", 7)
	if w.Code != 1234 || w.Msg != "测试错误: 参数 7" {
		t.Fatalf("With: %+v", w)
	}
	if e.Error() != "[1234] 测试错误" {
		t.Fatal("With 不应修改原错误")
	}
}
