# 工作项核验索引（题目 02：宠物门店的预约、接待、服务与取消退款）

> 本文件按《Go项目业务场景题.md》第 9 节要求，保留 feat/fix 提交的工作项关联与 meegle 核验记录。

## 提交与工作项对应表

| 提交 | 类型 | 版本号 | 工作项编号 | 描述 | 核验状态 |
|---|---|---|---|---|---|
| （本次 feat 提交，见 git log） | feat | v0.1.0 | #001 | 实现宠物门店预约、支付、核销、取消退款全链路 | ⚠️ 待补验 |

## meegle 核验记录

- 编号来源：学生确认，老师发放的飞书项目（r17te9）需求编号为递增序号，题目 02 对应 `001`。
- 核验命令（规范原文）：
  ```bash
  meegle workitem get --work-item-id 001 --project-key r17te9
  ```
- 实际执行记录（2026-11-12，北京时间）：
  1. `meegle-cli@0.3.0` 已通过 `npm install -g meegle-cli` 安装成功（本机可执行 `meegle --help`）。
  2. 执行核验返回：`[CONFIG_ERROR] 未找到 profile "default"，请先执行 meegle auth init --target-profile default ...`（exit=2）。
  3. 失败原因：本机缺少飞书项目开放平台插件凭证（`MEEGLE_PLUGIN_ID` / `MEEGLE_PLUGIN_SECRET` 及默认 userKey），未随题面材料发放到本机。
- 结论与整改：**核验未完成，该 feat 提交暂不视为合规**。按规范如实记录，不伪造"查询成功"。取得插件凭证后将立即执行：
  ```bash
  meegle auth init --plugin-id <ID> --plugin-secret <SECRET> --default-user-key <userKey>
  meegle space types --project-key r17te9
  meegle workitem get --type-key <type> --id 001 --project-key r17te9
  ```
  并将完整核验输出追加到本文件、更新上表核验状态。

## 其他提交（编号可选类型）

| 提交 | 类型 | 版本号 | 说明 |
|---|---|---|---|
| 5be0780 | docs | — | 补充已验证的安装构建测试命令与技术栈说明 |
| 67f6b47 | opt | v0.1.0 | 统一覆盖率统计口径为全 internal 包并固化 make cover |
| dc91b34 | opt | v0.1.0 | 新增 GitHub Actions 流水线与 Dockerfile、compose 部署编排 |
| （CI 产物提交） | opt | v0.1.0 | CI 上传覆盖率原件供审计取用 |

## 分支、PR 与合并记录

| 项 | 值 |
|---|---|
| 开发分支 | `zhou` |
| 主分支 | `main`（仓库默认分支） |
| PR #1 | `https://github.com/zm1464688361-svg/pa/pull/1`（zhou → main，13 个提交），合并提交 `ae5075232ec682300d9ebaa42a74dcb6dc777464` |
| PR #2 | 报表字段纠偏 + 越权拦截 + 端到端验收脚本，3 提交，合并提交 `e6f71121b023ce47ba0caf97c65256df115bce8a` |
| PR #3 | 缺陷记录与验收结果登记，1 提交，合并提交 `a9133905b18f8dad3d61ef3b870b205a2598cc47`（该提交的 CI 暴露并发迁移缺陷，由 PR #4 修复） |
| PR #4 | 工程骨架修复（页脚时钟 / 并发迁移锁 / Makefile 目标），合并提交 `7713bc8e0ad09ee45590dbf9816c03677cf60ab1` |
| CI | `check`（gofmt/vet/race/覆盖率≥85%）与 `docker`（镜像构建）在每次合并前均全绿 |

CI 失败修复过程（保留真实循环记录）：

1. `123b195` 首轮 CI：`gofmt` 步骤失败（本地未跑 `gofmt -l .`）→ 修复提交 `ea0da2c`。
2. `ea0da2c` 第二轮：`coverage` 步骤失败，日志 `open .../coverage/cover.out: no such file or directory`（CI 环境无 `coverage/` 目录）→ 修复提交 `cd81b80`（Makefile 与 CI 均加建目录）。
3. `cd81b80` 第三轮：全部通过。

## 待办（交付完整性）

- [ ] meegle 插件凭证到位后补核验 #001，更新本表
- [x] 推送 `zhou`、创建 PR #1 并合并 `main`
- [x] PR #2：报表字段/越权拦截修复 + 端到端验收脚本（合并提交 `e6f7112`）
- [x] PR #4：工程骨架修复与 AGENTS.md 实测更新（合并提交 `7713bc8`）
- [ ] 讲解效果视频录制与群提交（学生本人完成，登记时间点）
- [x] 覆盖率逐包达标（biz 87.2%、repo 88.9%，补 `coverage_gap_test.go`/`repo_gap_test.go`）
- [x] 明文口令从 `scripts/e2e.ps1` 移除（改读 `MYSQL_PWD`/`config.local.yaml`）；口令轮换已执行（见缺陷-11）
- [x] CI 增加 `deploy` job + `scripts/smoke.sh`（compose 部署、健康检查、业务冒烟、制品上传）
- [x] `deploy` job 首次在 `main` 真实运行：PR #6 合并提交 `086f961` 上 check/docker/**deploy 全绿**（compose 部署 + 健康检查 + smoke.sh 业务冒烟 + deploy-record 制品），运行链接 `https://github.com/zm1464688361-svg/pa/actions/runs/43203449605`
- [ ] 教学验收环境地址确认（当前部署到 runner 本地，无公网验收地址）
- [x] 覆盖率逐包门禁（covergate）随 CI 强制执行：合并提交 `086f961` 逐包 biz 87.2% / repo 85.7% / paysim 85.0%，整体 87.6%（CI 制品 coverage-profile 可回溯）
- [x] 自编 Skill 两个 + `docs/skill-usage.md` 调用记录
- [ ] `main` 分支保护：GitHub API 返回 403 `Upgrade to GitHub Pro or make this repository public`（私有仓库 + 免费套餐不支持），需教师确认仓库权限或转公开后再配置
- [ ] 会话导出与 `docs/sessions/` 索引（导出方式见 `docs/prompt-log.md` 第 3 节）

## 缺陷记录（未登记飞书 Bug 单，类型降级为 opt，待补 Bug 编号）

| 编号 | 发现方式 | 现象 | 修复提交 | Bug 单状态 |
|---|---|---|---|---|
| 缺陷-1 | `scripts/e2e.ps1` 访问 `/owner` | 店长报表页 500：模板引用 `CreatedAt`，且 IDL 未下发 `completedFen/noShowCount/noShowFen` | `ca5ec6c`（PR #2） | 待补登记 |
| 缺陷-2 | 同上，主人越权访问 `/owner`、`/support` 等 | 只读页仅判 RPC 传输错误，业务错误 Base.Code≠0 被静默放行，越权者看到空列表而非拒绝 | `ca5ec6c`（PR #2） | 待补登记 |
| 缺陷-3 | 骨架验证：`POST /clock offset_sec=7200` 后比对页脚 | 页脚「当前教学时间」取网关本机 `time.Now()`，教学时钟偏移对页面不生效（拨 +2h 页脚仍是真实时间） | `c2462f0`（PR #4） | 待补登记 |
| 缺陷-4 | CI `check` 的 race 步骤真实失败 | 多包并行测试共用 `petshop_test` 库，并发 AutoMigrate 报 `Error 1050 Table already exists`；本地因时序侥幸通过 | `c2462f0`（PR #4） | 待补登记 |
| 缺陷-5 | 骨架验证：核对 Makefile 与 help 输出 | `help` 宣称的 `run-all` 目标不存在；`clean` 使用 `rm -rf` 在 Windows cmd 下必失败 | `c2462f0`（PR #4） | 待补登记 |
| 缺陷-6 | 复检：逐包覆盖率核对 | 整体 85.5% 达标但 `biz 84.9%`、`repo 77.8%`，未满足"各核心包分别 ≥85%" | 本轮工作区（`coverage_gap_test.go`、`repo_gap_test.go`） | 待提交/合并 |
| 缺陷-7 | 复检：敏感信息扫描 | `scripts/e2e.ps1:14` 内置 MySQL root 明文口令并已合入 main | 本轮工作区（改读 `MYSQL_PWD`/`config.local.yaml`） | 待提交/合并；口令轮换待执行 |
| 缺陷-8 | 复检：流水线核对 | `ci.yml` 只有 check/docker，无部署、健康检查与业务冒烟，无法证明"自动部署上线" | 本轮工作区（新增 `deploy` job + `scripts/smoke.sh`） | 待提交/合并 + 首次运行 |
| 缺陷-9 | 审计独立复跑端到端（paysim 重启场景） | 支付通知幂等键仅用全局 `notify_id`：模拟渠道进程重启后通知号（如 N1-1）复用，新订单支付回调被静默判"重复通知"（返回 code 0），预约永久停留待支付——渠道重启后全部收款失效 | `76555e6`（幂等键改为 订单号+通知号 复合唯一，迁移兼容清理旧全局索引，补跨订单复用回归测试） | 已合并 `086f961`；paysim 全新启动（计数器归 1）复跑 e2e **16/16 通过**，修复得证 |
| 缺陷-10 | 审计复跑端到端（脏库 + paysim 重启） | 异常支付台账 `abnormal_payments.channel_trade_no` 全局唯一索引 + 渠道重启后交易号（如 T100000）复用 → 关单订单的迟到支付登记台账时报 1062 冲突（响应 code 5000），台账缺失且无支付流水——渠道重启后异常支付全部漏登记。另发现 e2e 脚本两处脚手架问题：dsn 带引号解析失败、NewSlot `LIMIT 1` 无排序在脏库复跑时命中历史同时段 | 本轮（台账幂等键改 订单号+交易号 复合唯一 + 登记幂等护栏 + 迁移清理旧索引 + 回归测试；脚本正则兼容引号、NewSlot 按 id 倒序取新建时段） | 待提交/合并复验 |
| 缺陷-11 | 口令轮换执行记录 | 本机 MySQL root 旧口令已在多轮测试与聊天记录中暴露 | 已执行 `ALTER USER root@localhost`，`config.local.yaml`（不入库）同步更新，服务重启 + e2e 16/16 复验通过 | 已完成（口令仅存本机不入库） |
| 缺陷-12 | 复评审计（2026-11-12 20:28/20:30/20:42 三次复现） | `TestDailyReport` 依赖运行时刻：`start=Clock.Now()+26h` 且第二时段 `+2h`，当日 20:58 后运行 `start+2h` 跨午夜，`day3≠day`，`cancel_test.go:544` 断言失败——用例晚间必失败、不可重复执行 | 本轮（基准时刻固定为次日 10:00，+3h 不再跨天；见下方第三轮整改记录） | 待提交/合并复验 |

> 说明：按 `commit-convention.md`，`fix` 必须关联真实 Bug 编号；上述缺陷尚未在飞书项目登记 Bug 单（本机无 meegle 凭证），故修复提交使用 `opt` 类型并在 PR 描述中写明缺陷详情，取得凭证后补登记 Bug 并回填编号。

## 工程骨架验证（2026-11-12）

对照 `docs/plan.md` Task 1 逐条实跑，完整命令与输出摘要记录在 `AGENTS.md` 的「骨架验证记录」表：

| 项 | 结果 |
|---|---|
| Go module | `go mod verify` → all modules verified（go1.27.1） |
| 构建 | `make build` → service/gateway/paysim/seed 四个可执行文件 |
| 静态检查 | `make vet` 退出码 0；`gofmt -l .` 无输出 |
| 单测 | `make test` 8 包全绿（`-p 1` 串行） |
| 覆盖率 | `make cover` → 85.5%（门禁 85%） |
| seed 幂等 | 连续执行两次不重复插入（users=6 pets=3，演示时段按 start_time 去重） |
| Hertz 网关 | 8081 `/login` 200；`/slots` 渲染次日时段与名额 |
| Kitex RPC | 页面数据经 9001 RPC 返回（非网关直连 DB） |
| MySQL | 业务库 `petshop`、测试库 `petshop_test` 迁移与用例均通过 |
| 教学时钟 | `POST /clock offset_sec=7200` → 页脚与页面时间 +2h、`clock_offsets` 持久化，置 0 复位 |
| race | 本机无 gcc 无法执行，由 CI ubuntu 执行并通过（`2b59e59` 两轮全绿） |

## 端到端验收（2026-11-12）

- 脚本：`scripts/e2e.ps1`；记录：`docs/verification.md`（16 个场景全部通过）
- 覆盖：四角色登录、预扣名额、支付回调确认、≥2h 自助取消、退款释放名额与重复回调幂等、<2h 人工审核、审核期拦截核销、核销/开始/完成强制顺序与幂等、关单后支付入异常台账、异常退款不释放名额、未到店需时段结束后手动标记、店长日清报表与主人越权拦截

## 整改提交清单（zhou 分支，2026-11-12 第二轮）

| 提交 | 类型 | 内容 | 对应整改项 |
|---|---|---|---|
| `84a7da3` | fix | 移除端到端脚本内置数据库口令，改读 `MYSQL_PWD`/`config.local.yaml` | 禁止密钥入库（历史提交仍含旧口令，须轮换） |
| `22b0aef` | test | 补 biz/repo 异常分支与并发用例 | 逐包覆盖率达标 |
| `9398f93` | feat | 新增 `tools/covergate` 逐包门禁与 `cover-pkgs`/`cover-strict` 目标 | 覆盖率门禁只报整体 |
| `434c228` | feat | CI 增加 main 自动部署、健康检查、业务冒烟与回退步骤，附 `scripts/smoke.sh`、`docs/deploy.md` | 自动发布与上线冒烟 |
| `c91786f` | feat | 自编 Skill：`biz-consistency-check`、`acceptance-check`，附 `docs/skill-usage.md` | Skill 总结与调用记录 |
| `ead6b4e` | docs | AI 使用说明、结构化事件清单、提示词导出说明 | 过程可追溯 |
| `e24d35b` | docs | AGENTS.md 与本文档登记整改项与待办 | 约束同步 |
| `4a4dccf` | opt | `.gitattributes` 统一 LF，消除 Windows 工作区 gofmt 误报 | 静态检查一致性 |

本地复跑结果（提交后）：`go vet ./...` 退出码 0、`gofmt -l .` 无输出、`go test ./internal/... -p 1` 9 包全绿、`make cover` 整体 87.9% 且 biz 87.2%/repo 88.9%/paysim 85.0%/clock 87.5% 逐包达标。

待办（未完成，不隐瞒）：`deploy` job 需合并到 main 后才会运行，运行结果与 `deploy-record` 制品尚未产生；讲解视频与当日群提交未完成；meegle 工作项核验仍缺凭证；`main` 分支保护受 GitHub 套餐限制（403）。

## 整改提交清单（zhou 分支，2026-11-12 第三轮）

| 提交 | 类型 | 内容 | 对应整改项 |
|---|---|---|---|
| （本轮提交，见 git log） | opt | `TestDailyReport` 基准时刻固定为次日 10:00，消除运行时刻依赖（缺陷-12） | 复评审计：测试可重复性 |

复跑记录（2026-11-12 21:33，处于原"20:58 后必失败"窗口）：

1. `go test ./internal/biz/ -run TestDailyReport -v` → PASS（第一次，21:33）。
2. `go test ./internal/biz/ -run TestDailyReport` → ok（第二次，21:33）。
3. `go test ./internal/... -count=1 -p 1` → 8 包全 ok；`gofmt -l .` 无输出（仅未入库 `tools/covpack/main.go` 除外）；`go vet ./...` 退出码 0。

说明：本轮修复由 AI 在用户明确指令下执行，如实登记于此；meegle 凭证仍缺，缺陷-12 未登记飞书 Bug 单，类型用 `opt` 并在 PR 描述写明缺陷详情。视频/群提交/会话导出/Token 台账/MCP 建仓记录/教师侧发布记录经复评核实仍未补齐（检索 0 结果），不伪造。

## 整改提交清单（zhou 分支，2026-11-12 第四轮：补齐端到端演示缺口）

对应复评审计指出的两处"仅单测覆盖、未入 e2e"缺口（02-A1-10/11）与重启持久化缺口（02-A4-04）：

1. **e2e 场景 7（并发抢位→释放→再约）**：容量 1 的时段上 user1/user2 双进程并发抢位，断言仅一人成功（预约数=1、余量=0）；得主关单释放（余量=1）；另一用户再约成功（余量=0）。
2. **e2e 场景 8（退款失败→重试→终态）**：≥2h 自助取消后模拟渠道连续 3 次退款失败回调（retry_count=1→2→3），客服两次重试后第三次触发终态 `final_failed` 转人工台账；终态后再重试被拒、状态保持、预约不复活。
3. **重启持久化验证（2026-11-12 22:0x，cf92c15 二进制）**：重启三进程前后快照比对完全一致——时段 122 个/余量合计 331、预约状态分布（cancelled 32 / closed 35 / completed 15 / confirmed 1 / no_show 15 / pending_payment 1）、终态退款 1、异常台账 13，重启后全部不变且 `/login` 200。

复跑记录（2026-11-12 22:00 前后，main=cf92c15 构建）：

1. `scripts/e2e.ps1` 24 步全部通过（含新增场景 7/8 共 8 步），结果写入 `docs/verification.md`（生成时间见该文件）。
2. 重启健康检查：8081/9001/9002 全部监听，`/login` 200。
3. 重启前后快照比对：一致（见上）。

说明：本轮验证与脚本扩充由 AI 在用户明确指令下执行并如实登记；讲解视频可沿 `docs/verification.md` 场景顺序录制演示。
