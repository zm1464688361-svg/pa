// 业务不变量核对工具：配合 skills/biz-consistency-check 的动态校验步骤使用。
// 用法：go run ./tools/invariantcheck（读取 TEST_DB_DSN 或 config.local.yaml 的 test_dsn）
// 退出码：0=一致，1=发现不一致，2=环境不可用。
package main

import (
	"fmt"
	"os"

	"github.com/zm1464688361-svg/pa/internal/config"
	"github.com/zm1464688361-svg/pa/internal/repo"
)

func main() {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = config.LoadEnvKey("test_dsn")
	}
	if dsn == "" {
		fmt.Println("未配置测试库，跳过动态校验")
		os.Exit(2)
	}
	db, err := repo.Open(dsn, false)
	if err != nil {
		fmt.Println("连接失败:", err)
		os.Exit(2)
	}

	type row struct {
		ID        int64
		Capacity  int32
		Remaining int32
		Effective int64
	}
	var rows []row
	if err := db.Raw(`SELECT s.id, s.capacity, s.remaining,
		(SELECT COUNT(*) FROM appointments a WHERE a.slot_id = s.id AND a.status IN ('pending_payment','confirmed')) AS effective
		FROM time_slots s
		WHERE s.remaining <> s.capacity - (SELECT COUNT(*) FROM appointments a WHERE a.slot_id = s.id AND a.status IN ('pending_payment','confirmed'))`).Scan(&rows).Error; err != nil {
		fmt.Println("名额一致性查询失败:", err)
		os.Exit(2)
	}
	fmt.Printf("名额不一致时段数: %d\n", len(rows))
	for _, r := range rows {
		fmt.Printf("  slot=%d capacity=%d remaining=%d effective=%d\n", r.ID, r.Capacity, r.Remaining, r.Effective)
	}

	var noShowReleased int64
	db.Raw(`SELECT COUNT(*) FROM appointments WHERE status = 'no_show' AND slot_released = 1`).Scan(&noShowReleased)
	fmt.Printf("未到店却标记返还名额（应为 0）: %d\n", noShowReleased)

	var refundedNoFlag int64
	db.Raw(`SELECT COUNT(*) FROM appointments a JOIN refunds r ON r.booking_id = a.id
		WHERE r.status = 'success' AND a.status = 'cancelled' AND a.slot_released = 0`).Scan(&refundedNoFlag)
	fmt.Printf("退款成功但未标记返还名额（应为 0）: %d\n", refundedNoFlag)

	if len(rows) > 0 || noShowReleased > 0 || refundedNoFlag > 0 {
		os.Exit(1)
	}
	fmt.Println("结论：名额与有效预约一致")
}
