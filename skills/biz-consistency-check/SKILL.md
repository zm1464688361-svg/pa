---
name: biz-consistency-check
description: 检查宠物门店预约系统的业务不变量是否被代码遵守——名额扣减/返还一致性、退款幂等、宠物互斥、核销顺序、教学时钟、越权拦截。修改 biz/repo/gateway 代码或新增用例后调用。
---

# 业务不变量检查（题目 02）

## 用途

用可执行的检查代替"看代码猜逻辑"，确认 8 条硬约束没有被后续修改破坏：

1. 名额预扣/释放成对出现，且返还仅一次（`slot_released` 幂等标记）；
2. 时段开始后不可预约、过期时段自动关闭；
3. 同一宠物时间区间重叠拦截（仅待支付/已支付算有效）；
4. 待支付 5 分钟超时关单并释放名额，计时起点=订单创建时间；
5. 取消分流：≥2h 自助、<2h 人工审核，审核期锁定名额且拦截核销；
6. 核销→开始→完成顺序强制，三步幂等，不重复累加营收；
7. 退款最多 3 次、失败终态、异常支付退款不释放名额不恢复预约；
8. 所有时间判断走 `Clock`，禁止业务代码直接 `time.Now()`；所有写操作校验角色与归属。

## 触发场景

- 改动 `internal/biz`、`internal/repo`、`cmd/service` 后；
- 新增/调整状态流转或新增接口后；
- CI 覆盖率或 race 失败、线上出现名额/状态不一致时。

## 输入

- 待检查的代码版本（git 提交哈希）；
- 可连接的测试库 DSN（`TEST_DB_DSN`），用于 SQL 侧核对；
- 可选：一次端到端运行后的库快照。

## 执行步骤

1. **静态扫描**
   ```bash
   # 时间判断是否绕过教学时钟
   grep -rn "time.Now()" internal/biz internal/repo | grep -v "_test.go" | grep -v "updated_at"
   # 名额变更是否只有预扣/释放两处入口
   grep -rn "remaining" internal/biz/*.go | grep -v "_test.go"
   # 幂等护栏
   grep -rn "slot_released\|RowsAffected" internal/biz/*.go | grep -v "_test.go"
   ```
   预期：`time.Now()` 仅出现在审计/`updated_at`，业务分支均用 `b.Clock.Now()`；名额写操作只出现在 `CreateBooking`（-1）、`releaseSlot`（+1，带 `remaining < capacity` 护栏）。

2. **状态机核对**：逐个打开 `booking.go`、`cancel.go`、`fulfill.go`、`noshow.go`、`refund.go`，核对每个 `switch`/`if` 是否覆盖题面列出的合法与非法状态，非法状态必须返回业务错误码而非静默成功。

3. **权限核对**：对每个导出方法确认首行是 `requireRole` 或显式归属校验；页面层确认业务错误 `Base.Code != 0` 也被拦截（不能只看 RPC 传输错误）。

4. **SQL 侧一致性**（测试库）
   ```sql
   -- 名额 = 容量 - 有效预约数（有效=待支付/已支付）
   SELECT s.id, s.capacity, s.remaining,
          (SELECT COUNT(*) FROM appointments a WHERE a.slot_id=s.id AND a.status IN ('pending_payment','confirmed')) AS effective
   FROM time_slots s
   WHERE s.remaining <> s.capacity - (SELECT COUNT(*) FROM appointments a WHERE a.slot_id=s.id AND a.status IN ('pending_payment','confirmed'));
   -- 应返回空集
   -- 已取消/已退款单不得标记为已返还名额
   SELECT id, status, slot_released FROM appointments WHERE status IN ('cancelled') AND slot_released = 0;
   ```
   第二条对"退款处理中"为正常，需结合退款单状态判断，不直接判失败。

5. **跑验证**：`go test ./internal/biz/... -race -count=1 -p 1`，关注并发用例（最后名额、重复回调、核销与取消并发）。

## 输出格式

| 检查项 | 证据（命令输出/文件:行） | 结论（通过/违规/无法判定） | 建议修复 |
|---|---|---|---|
| 名额一致性 | `...` | 通过 | — |
| 幂等护栏 | `...` | 违规 | 在 X 处加条件更新 |

必须给出命令原文与输出摘要，不能只写"已检查"。

## 失败处理

- 静态扫描命中业务代码 `time.Now()`：判定违规，改为 `b.Clock.Now()` 并补时间边界用例。
- SQL 返回非空：先确认是否处于"退款处理中"（名额合法锁定），否则记录为缺陷并定位到具体状态迁移函数。
- 测试库不可用：只能给出静态结论，输出必须标注"动态校验未执行"，禁止当作通过。

## 完成标准

8 条不变量全部有"命令/文件位置 + 输出"证据；违规项已修复并复跑通过；无一项以"人工确认"代替执行证据。
