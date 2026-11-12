# 题目 02 宠物门店预约系统 — 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.
>
> **Goal:** 交付宠物门店"预约→支付→核销→服务→取消退款→日清"完整闭环系统（Go + Hertz + Kitex + MySQL）。
>
> **Architecture:** Hertz HTTP 网关（页面+JSON）→ Kitex RPC（Thrift）核心业务服务 → MySQL（GORM）；独立 pay_sim 模拟支付渠道进程；教学时钟统一取时。
>
> **Tech Stack:** Go 1.27、Hertz、Kitex、Thrift（IDL）、GORM、MySQL 8.0、go test（覆盖率≥85%、-race）。

**约束来源**：`AGENTS.md`（角色权限矩阵/时段预约硬约束/红线）、`docs/requirements.md`、`commit-convention.md`（提交格式 `[类型] [版本] [#编号] 中文描述`）。

---

## 任务清单（依赖顺序，每项含完成条件）

### Task 1: 工程骨架与基础设施
**Files:** `go.mod`, `idl/*.thrift`, `cmd/gateway/main.go`, `cmd/service/main.go`, `cmd/paysim/main.go`, `cmd/seed/main.go`, `internal/clock/`, `internal/model/`, `internal/repo/`, `scripts/init.sql`, `Makefile`
- [x] 步骤 1: `go mod init`, 引入 hertz/kitex/gorm 依赖，`go mod tidy` 通过
- [x] 步骤 2: `scripts/init.sql` 建库建表（见数据模型）；`cmd/seed` 预置数据可重复执行（幂等）
- [x] 步骤 3: `internal/clock`：Clock 接口 + DB 偏移实现 + HTTP 调整接口；单测覆盖偏移计算
- [x] 步骤 4: Kitex 冒烟：`kitex -h` 可用，生成代码可编译（Windows 兼容验证，失败则上报降级）
- [x] 步骤 5: Makefile：`make build/test/cover/race/seed/run` 全部真实可执行；写入 `AGENTS.md` 安装/构建/测试节
**完成条件**: `make build` 产出三个可执行文件；`make test` 通过；AGENTS.md 技术栈节更新。

### Task 2: 数据模型与仓储层
**Files:** `internal/model/*.go`, `internal/repo/*.go`, 对应 `*_test.go`
表：`users, pets, shops, staff, service_items, time_slots, appointments, payments, refunds, cancel_requests, abnormal_payments, no_show_marks, clock_offset, audit_logs`
- 关键约束：`appointments` 上 `(pet_id, slot_id)` 有效状态部分唯一由业务层保证 + DB 事务行锁；`time_slots.remaining` 原子扣减。
- [x] 失败测试先行（GORM AutoMigrate 一致性、字段默认值）
- [x] 实现 + 集成测试（testcontainers 不用，直接连本地 MySQL `petshop_test` 库）
**完成条件**: `go test ./internal/repo/... -race` 通过。

### Task 3: 时段与预约核心（含并发）
**Files:** `internal/biz/slot.go`, `internal/biz/booking.go`, `*_test.go`
- [x] 发布时段校验（起止时间、名额>0）；时段过期自动关闭（惰性判定）
- [x] 预约资格校验链：名额余量 → 时段已开始 → 宠物归属 → 同宠物重叠时段锁 →（原子预扣名额）
- [x] 待支付 5 分钟超时关单释放（幂等）+ 主人主动取消
- [x] 并发测试：20 并发抢 1 名额仅 1 成功；同宠物并发预约仅 1 条有效单（`-race`）
**完成条件**: 上述测试全绿；失败原因文案与原型一致。

### Task 4: 支付与异常支付
**Files:** `internal/biz/payment.go`, `internal/biz/abnormal.go`, `internal/biz/refund.go`, `*_test.go`
- [x] 回调鉴权（签名）+ 订单/金额匹配；重复通知幂等；同订单多笔支付第一笔有效其余入异常台账
- [x] 关单后迟到支付 → 异常支付列表，不恢复预约不释放名额
- [x] 退款发起（客服）→ 渠道回调成功/失败；释放名额幂等一次；7 自然日回调超时（教学时钟）；重试≤3 → 终态失败
**完成条件**: 全部分支单测覆盖（成功/失败/重复/迟到/超时/重试耗尽）。

### Task 5: 取消两分支与审核
**Files:** `internal/biz/cancel.go`, `*_test.go`
- [x] ≥2h 自助取消（全额退款）；<2h 必填原因提交客服审核
- [x] 审核期间：名额锁定 + 禁止核销（核销前置校验查"审核中"申请）
- [x] 批准 → 退款；拒绝 → 写明原因 + 解锁回 CONFIRMED
- [x] 并发：取消提交 vs 核销，先落库者定结果（DB 锁）
**完成条件**: 分支测试 + 并发测试（`-race`）通过。

### Task 6: 核销履约与未到店
**Files:** `internal/biz/fulfill.go`, `internal/biz/noshow.go`, `*_test.go`
- [x] 核销前置校验（本店/已确认/无审核中取消）；核销→开始→完成 强制顺序、三步幂等
- [x] 重复核销不重复记收入；已核销/服务中/已完成拒绝取消
- [x] 未到店：时段结束后店长可见标记入口；原因必填、可编辑、状态不可回退
**完成条件**: 顺序违规、重复操作、跨店核销均有明确拒绝结果与测试。

### Task 7: 日清统计
**Files:** `internal/biz/report.go`, `*_test.go`
- [x] 当日按时段明细（有效/已完成/取消/未到店/退款待处理），按时段开始时间归属自然日
- [x] 已收款/已退款/实收/退款待跟进指标；未到店单列不计入已完成服务收入
**完成条件**: 构造混合日数据，指标与手工核算一致（测试断言）。

### Task 8: Kitex RPC 服务与 Hertz 网关
**Files:** `idl/appointment.thrift`, `cmd/service/`, `cmd/gateway/`, `internal/http/*/`（按角色分包）
- [x] IDL 定义 RPC 接口；kitex 生成；service 实现 biz 装配；gateway 通过 kitex client 调用
- [x] 会话鉴权（签名 Cookie，预置账号选择登录）；越权访问返回明确错误（越权测试）
- [x] 四角色页面按 `docs/prototype.md` P1–P20 实现（Go html/template，墨绿主题轻量还原）
**完成条件**: 手工走查 P1–P20 全部可操作；越权用例测试通过。

### Task 9: pay_sim 模拟支付渠道
**Files:** `cmd/paysim/`, `internal/paysim/`
- [x] 受理支付单/退款单；管理页按钮：支付成功/失败/不回调/重复回调/退款成功/失败
- [x] 异步 HTTP 回调业务服务（带签名）；可配置延迟（演示迟到）
**完成条件**: 五种触发均产生题面要求的业务结果。

### Task 10: 单元测试与覆盖率门禁
**Files:** `Makefile`, `scripts/cover.sh`
- [x] `go test ./... -race -coverprofile`；核心业务包（internal/biz, internal/repo, internal/clock, internal/paysim）各 ≥85%
- [x] 覆盖率原始输出与 HTML 报告留档 `coverage/`
**完成条件**: 覆盖率门禁脚本 exit 0。

### Task 11: 自编 Skill
**Files:** `.codebuddy/skills/petshop-business-check/`, `.codebuddy/skills/petshop-acceptance/`, `skill-usage.md`
- [x] 业务专项 Skill：校验名额/时间/取消政策/履约进度/退款约束（可执行检查清单）
- [x] 验收检查 Skill：对照题面五组场景逐项核对 + 证据位置
- [x] `skill-usage.md` 按题面表格逐项填写；保留"检查发现问题→修正→复检"记录
**完成条件**: 两个 Skill 实际调用过至少一次并留有输出记录。

### Task 12: CI/CD 与部署
**Files:** `.github/workflows/ci.yml`, `Dockerfile`, `docker-compose.yml`, `scripts/deploy.*`, `docs/deploy.md`
- [x] 流水线：lint（gofmt/vet）→ 测试 → 覆盖率门禁 → 构建 → 部署 → 健康检查 → 冒烟
- [x] 部署记录、版本标识、回滚说明（恢复上一版本方法）
**完成条件**: 一次真实部署 + 冒烟结果留档。

### Task 13: PR 合并与交付
- [x] `zhou` → `main` PR，附测试/覆盖率/验收证据；main 禁推保护
- [x] 最终交付清单核对（题面 一.7）
**完成条件**: PR 合并记录 + 部署提交一致。

---

## 提交规范执行说明

- 格式：`[feat] [v0.1.0] [#需求编号] 中文描述`（docs/test/opt 不强制编号）。
- meegle CLI 与飞书项目当前不可用（假设 A5）：feat/fix 编号待教师提供后再补核验记录；期间按阶段使用 `docs`/`test`/`opt` 提交推进，所有 feat 级变更集中登记在 `docs/workitems.md` 待补编号。
- 一个提交只做一件事；禁止提交密钥（MySQL 密码走环境变量 `DB_DSN` 或 `config.local.yaml`，已加入 `.gitignore`）。

## TDD 执行方式

每个业务任务先写失败测试（表驱动 + 明确断言文案）→ 实现 → `go test ./internal/... -race` 通过 → 提交。并发场景必须可重复执行（循环 N 次或 goroutine 栅栏）。
