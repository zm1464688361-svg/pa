package clock

import (
	"testing"
	"time"
)

func TestClockOffset(t *testing.T) {
	c := New(nil)
	teach := c.Now()
	if diff := teach.Sub(time.Now()); diff > 2*time.Second || diff < -2*time.Second {
		t.Fatalf("无偏移时教学时钟应接近真实时间: %v", diff)
	}
	if err := c.SetOffset(3600); err != nil {
		t.Fatalf("nil store 应可直接设置: %v", err)
	}
	if c.Now().Sub(time.Now()) < 59*time.Minute {
		t.Fatalf("偏移应生效")
	}
	if c.Offset() != 3600 {
		t.Fatalf("Offset(): %d", c.Offset())
	}
	c.SetOffset(-60)
	if c.Now().After(time.Now().Add(30 * time.Second)) {
		t.Fatal("负偏移应回拨")
	}
	c.SetOffset(0)
	if c.Offset() != 0 {
		t.Fatal("恢复真实时间后偏移应为 0")
	}
}

// DBStore 由 biz 集成测试（真实 MySQL）覆盖；此处保证编译期满足接口。
func TestDBStoreImplements(t *testing.T) {
	var _ OffsetStore = (*DBStore)(nil)
}
