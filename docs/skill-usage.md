# Skill 使用清单（题目 02）

> 本文件只登记**实际调用过**的 Skill。调用记录对应 2026-11-12 的修复与复检过程，命令输出见文末。

## 1. 自编 Skill：biz-consistency-check

| 说明项 | 内容 |
|---|---|
| 名称、来源与版本 | `biz-consistency-check`，自编，v1.0（2026-11-12），文件 `skills/biz-consistency-check/SKILL.md` |
| 使用位置 | Agent 测试阶段：补齐覆盖率用例后，核对 8 条业务不变量是否仍成立；对应本文件第 3 节命令输出 |
| 使用原因 | 新增测试大量走异常分支，需要确认"补测试"没有掩盖真实业务缺陷（例如名额多释放、PowerShell/时钟绕过），仅看覆盖率高不能证明规则正确 |
| 使用过程 | 输入：提交 `fa44b06` 后的工作区 + 测试库 `petshop_test`；执行步骤 1（`time.Now()`/`remaining` 扫描）、步骤 3（权限首行核对）、步骤 4（名额一致性 SQL，由 `tools/invariantcheck` 落地） |
| 实际帮助 | ① 扫描出 `cancel.go:57`、`cancel.go:168` 的 `time.Now()`，逐行核对为取消单审计时间戳（业务判断在 `cancel.go:56` 用 `Clock.Now()`），判定不违规，避免了误改；② 确认名额写入口只有 `CreateBooking`（-1）与 `releaseSlot`（+1，带 `remaining < capacity` 护栏）；③ 动态校验返回 0 条不一致 |
| 局限 | 动态校验只覆盖当前库快照，无法证明历史数据；并发一致性仍依赖 `-race` 用例 |

## 2. 自编 Skill：acceptance-check

| 说明项 | 内容 |
|---|---|
| 名称、来源与版本 | `acceptance-check`，自编，v1.0（2026-11-12），文件 `skills/acceptance-check/SKILL.md` |
| 使用位置 | 提交 PR 前自检：审计报告指出 A3-02（逐包覆盖率）与 A4-02（部署流水线）不达标后，按该 Skill 逐条复检 |
| 使用原因 | 上一版只有"整体 85.5%"一个数字，掩盖了 `biz 84.9%`、`repo 77.8%` 未达逐包门禁；需要固定的三态结论表避免自评"基本通过" |
| 使用过程 | 输入：`coverage/cover.out`、`go test -cover` 输出、`git status`、`.github/workflows/ci.yml`；执行步骤 2/3/6（门禁、逐包覆盖率、密钥扫描） |
| 实际帮助 | ① 复检确认逐包不达标（biz 84.9%、repo 77.8%），据此补 `coverage_gap_test.go`（biz 87.2%）与 `repo_gap_test.go`（repo 88.9%）；② 命中 `scripts/e2e.ps1` 内置明文口令，改为读 `MYSQL_PWD`/`config.local.yaml`；③ 指出 `ci.yml` 缺部署与冒烟 job，据此新增 `deploy` job 与 `scripts/smoke.sh` |
| 局限 | 无法验证视频与群提交这类线下材料，只能标"待补证" |

## 3. 实际调用记录（2026-11-12，本机 Windows）

### 3.1 biz-consistency-check

```text
== time.Now in biz/repo (non-test) ==（节选）
internal\biz\biz.go:74:      "updated_at": time.Now()                  → 审计字段，合规
internal\biz\cancel.go:56:   urgent := s.StartTime.Sub(b.Clock.Now()) → 业务判断走 Clock，合规
internal\biz\cancel.go:57:   now := time.Now()                         → 取消单审计时间戳，合规
internal\biz\cancel.go:168:  now := time.Now()                         → 审核时间戳，合规

== remaining 写入口 ==
biz.go:86-87      Where("id = ? AND remaining < capacity").UpdateColumn("remaining", +1)  → 唯一释放入口，有护栏
booking.go:143-144 Where("id = ? AND remaining > 0").UpdateColumn("remaining", -1)         → 唯一预扣入口

$ go run ./tools/invariantcheck
名额不一致时段数: 0
未到店却标记返还名额（应为 0）: 0
退款成功但未标记返还名额（应为 0）: 0
结论：名额与有效预约一致
```

### 3.2 acceptance-check

```text
$ go test ./internal/... -count=1 -p 1 -cover（修复后）
ok internal/biz     87.2%    ← 修复前 84.9%（不达标）
ok internal/repo    88.9%    ← 修复前 77.8%（不达标）
ok internal/paysim  85.0%
ok internal/clock   87.5%   config 92.6%   session 95.0%   errs/model/sign 100.0%

$ git grep -n "asdfghjkl"   → 无输出（明文口令已移除）
$ 检查 ci.yml：deploy job 存在，含 health-check + scripts/smoke.sh 业务冒烟
```

## 4. 配置方式

- 自定义 Skill 放在仓库 `skills/<name>/SKILL.md`，用 YAML front matter 声明 `name` 与 `description`（description 写触发场景，供 Agent 判断何时加载）。
- 在 CodeBuddy 中把该目录作为项目级 Skill 目录加载，或直接引用文件路径让 Agent 读取后按步骤执行；本次是"读取 SKILL.md → 逐步执行 → 把命令输出回填本文件"。
- 检查类 Skill 必须包含可执行命令与失败处理，否则退回为普通提示词，不计入自编 Skill。

## 5. 未使用的既有 Skill

审计用的 `student-project-audit`（本地评测工具）不属于交付物，未登记为本题使用成果。
