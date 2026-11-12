# 自动部署与上线冒烟（题目 02）

## 1. 流水线结构

`.github/workflows/ci.yml` 三个 job，全部在 `main` 合并版本上执行：

| job | 内容 | 触发 |
|---|---|---|
| `check` | gofmt → vet → build → `go test -race -p 1` → 覆盖率门禁（整体 ≥85%，同时按包检查）→ 上传 `coverage-profile` 制品 | push / PR |
| `docker` | 分别构建 service、gateway 镜像，验证镜像可产出 | push / PR |
| `deploy` | compose 构建 → `up -d` → seed → 健康检查（网关 8081、RPC 9001、paysim 9002）→ 业务冒烟 → 上传 `deploy-record` → 结束清理 | 仅 `push` 到 `main` |

`deploy` 依赖 `check` 与 `docker` 成功，因此"质量门禁失败不得发布"由 `needs` 强制，不靠人工判断。

## 2. 部署步骤（deploy job 实际执行）

```bash
docker compose build
docker compose up -d mysql service gateway paysim
docker compose --profile tools run --rm seed     # 幂等种子数据
# 健康检查：轮询 http://127.0.0.1:8081/login 直到 200，再探 9001/9002 端口
scripts/smoke.sh http://127.0.0.1:8081
```

产物：`deploy-record.txt`（部署提交哈希 + 提交信息 + 时间）、`smoke.log`（每条冒烟结果），均作为 `deploy-record` 制品上传，可按 run 追溯。

## 3. 业务冒烟覆盖（scripts/smoke.sh）

| 步骤 | 断言 |
|---|---|
| 健康检查 | `GET /login` → 200 |
| 店长发布时段 | `POST /owner/slots` 后库内存在新时段 |
| 主人可见时段 | `GET /slots` 渲染包含「时段」 |
| 提交预约 | `POST /bookings` 后 `remaining` 由 3 → 2、`appointments` 新增 1 条 |
| 教学时钟 | `POST /clock 7200` 后 `clock_offsets.offset_sec=7200`，再复位为 0 |

任一 `SMOKE FAIL` 即 job 失败，本次发布判定为不成功。

## 4. 版本对应关系

- 部署版本 = `main` 上的合并提交（`DEPLOY_SHA`），与 `check` job 跑的是同一 commit；
- 制品 `deploy-record.txt` 记录该 SHA，可与 PR、CI run 一一对应；
- 开发分支 `zhou` 只跑 check/docker，不触发 `deploy`（`if` 限定 `refs/heads/main`）。

## 5. 回退方案

| 场景 | 操作 |
|---|---|
| 冒烟失败、需立即下线本次发布 | job 末尾 `if: always()` 的 `docker compose down -v` 已执行清理；生产环境等价操作为停掉新版本容器并删除新卷 |
| 回退到上一可用版本 | 记录上一可用版本的镜像 tag（compose 默认按当前源码构建，建议给镜像打 `pa-gateway:<sha>` 标签后保留），执行 `docker compose down` 后用上一 tag 重新 `up -d`，再跑一次 `scripts/smoke.sh` 确认 |
| 首次部署失败 | 直接 `docker compose down -v` 撤销本次发布，恢复为"未上线"状态，修复后重新走 PR + deploy |

## 6. 部署记录

> 按实际运行填写，未真实执行前不得写"已上线"。

| 项 | 值 |
|---|---|
| 首次部署运行（CI run） | 待 main 分支下一次 push 后由 `deploy` job 产生，填写 run 链接与 `deploy-record.txt` 中的 SHA |
| 部署环境 | GitHub Actions runner 上的 docker compose（教学验收环境地址待教师分配后替换为正式地址） |
| 部署版本 | 与 `main` 合并提交一致（待填） |
| 冒烟结果 | `smoke.log`（待填） |
| 回退演练 | `docker compose down -v` 已在每次 deploy 末尾执行 |

本地手工部署同样可用：

```bash
docker compose up -d --build
docker compose --profile tools run --rm seed
MYSQL_ROOT_PASSWORD=root bash scripts/smoke.sh http://127.0.0.1:8081
```

## 7. 已知限制

- 教学验收环境尚未分配，当前 `deploy` 部署到 runner 本地，**不对应可公网访问的验收地址**；地址到位后只需替换 `smoke.sh` 的目标 URL，流水线结构不变。
- 镜像未推送到镜像仓库，回退依赖本地/runner 缓存；如需长期回退能力，应加 `docker tag` + registry push。
