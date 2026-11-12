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
| PR | #1 `https://github.com/zm1464688361-svg/pa/pull/1`（zhou → main，13 个提交） |
| 合并提交 | `ae5075232ec682300d9ebaa42a74dcb6dc777464`（merge 方式，保留开发分支原始提交） |
| CI | `check`（gofmt/vet/race/覆盖率≥85%）与 `docker`（镜像构建）在合并前于 `6492121` 上全部通过 |

CI 失败修复过程（保留真实循环记录）：

1. `123b195` 首轮 CI：`gofmt` 步骤失败（本地未跑 `gofmt -l .`）→ 修复提交 `ea0da2c`。
2. `ea0da2c` 第二轮：`coverage` 步骤失败，日志 `open .../coverage/cover.out: no such file or directory`（CI 环境无 `coverage/` 目录）→ 修复提交 `cd81b80`（Makefile 与 CI 均加建目录）。
3. `cd81b80` 第三轮：全部通过。

## 待办（交付完整性）

- [ ] meegle 插件凭证到位后补核验 #001，更新本表
- [x] 推送 `zhou`、创建 PR #1 并合并 `main`
- [ ] 讲解效果视频录制与群提交（学生本人完成，登记时间点）
- [ ] 部署验证（本机 Docker Desktop 未启动；CI 已完成镜像构建验证，实际部署待验收环境执行）
