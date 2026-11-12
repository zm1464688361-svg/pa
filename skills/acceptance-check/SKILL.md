---
name: acceptance-check
description: 依据《Go项目业务场景题》与题目 02 原文，逐项核对交付物的可验证证据（覆盖率门禁、race、端到端记录、部署运行、Skill 与会话导出、Git 与工作项规范），输出带证据位置的验收表与结论。合并前或每日交付前调用。
---

# 题目 02 验收检查

## 用途

把"是否完成"从自述转为可核查证据：每条要求必须指向文件/命令/CI 运行/API 结果，缺证据即为不通过。

## 触发场景

- 提交 PR 前自检、每日交付前自检；
- 教师/复核方抽查；
- 修复缺陷后确认门禁是否重新达标。

## 输入

- 待验收的提交（主分支合并提交或 PR head）；
- 仓库可读权限（本地 git + GitHub API）；
- 可选的 CI/制品访问权限。

## 执行步骤

1. **版本对应**：`git log -1 --format='%H %ad %s'`；确认 PR 源提交 → 合并提交 → CI 运行 → 制品 是同一版本。
2. **质量门禁**：`gofmt -l .`、`go vet ./...`、`go build ./...`、`go test ./internal/... -race -count=1 -p 1`。
3. **覆盖率**：`make cover` 后 `go tool cover -func coverage/cover.out | tail -1`，并按包核对 `go test ./internal/<pkg> -cover`：整体与 `biz`、`repo` 等核心包分别 ≥85%。
4. **业务场景**：核对 `docs/verification.md` 是否覆盖题面五组场景与关键异常（超时关单、重复回调、审核期拦截、跨店核销、未到店、异常支付退款），每条要有库内数据或页面结果。
5. **部署**：CI 中 `deploy` job 是否在 main 上成功；`deploy-record` 制品中是否有提交哈希；`smoke.log` 是否全为 `SMOKE OK`；`docker-compose.yml` 能否 `up -d` 并通过健康检查。
6. **规范**：`main` 分支保护是否开启；提交信息是否符合 `commit-convention.md`（类型、版本号、工作项编号）；`git grep -nE "(password|passwd|pwd|secret|token)"` 是否命中真实凭据。
7. **过程材料**：`skills/` 下自编 Skill、`docs/skill-usage.md`、会话导出与 `docs/events.jsonl`、`docs/ai-usage.md` 是否存在且内容与提交对得上。

## 输出格式

| 要求 | 门槛 | 证据位置 | 结论 | 缺口 |
|---|---|---|---|---|
| 覆盖率 | 整体/核心包 ≥85% | `coverage/cover.out` total 行 | 通过/不通过 | … |
| 部署 | main 部署 job 成功 | CI run URL + 制品 | 通过/不通过/待执行 | … |

结论只能取"通过/不通过/待补证"三态，禁止"基本通过"。

## 失败处理

- 覆盖率不足：定位未覆盖函数（`go tool cover -func`），补正常+异常分支用例后重跑，禁止排除生产代码或删断言。
- 部署失败：保留失败日志与容器状态，回退到上一可用版本（`docker compose down` + 上一镜像 tag），修复后重新走 PR。
- 门禁未过而 PR 已合：视为流程违规，记录并在下一个 PR 补齐，不得修改历史掩盖。

## 完成标准

每条要求都有可点击/可复跑的证据；不存在"文档写了但跑不通"的项；缺失材料明确标注"待补证"及补证方式。
