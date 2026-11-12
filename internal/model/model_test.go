package model

import "testing"

func TestEffectiveStatuses(t *testing.T) {
	s := EffectiveStatuses()
	if len(s) != 2 || s[0] != StatusPendingPayment || s[1] != StatusConfirmed {
		t.Fatalf("有效预约状态 = 待支付+已支付: %v", s)
	}
}

func TestClockOffsetTableName(t *testing.T) {
	if (ClockOffset{}).TableName() != "clock_offsets" {
		t.Fatal("表名错误")
	}
}
