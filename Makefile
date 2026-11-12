# 宠物到店服务预约系统（题目 02）— 构建/测试/运行
# Windows 本机 + GitHub Actions 均可执行。

BIN   := bin

# 数据库 DSN：优先环境变量 DB_DSN，其次 config.local.yaml（不入库）
.PHONY: help build test cover cover-pkgs race vet fmt seed run run-all clean smoke invariant

help:
	@echo "make build   - 编译 service/gateway/paysim/seed 到 bin/"
	@echo "make test    - 全部单元测试"
	@echo "make cover   - 覆盖率（门禁：整体 ≥85%）"
	@echo "make cover-pkgs - 逐包覆盖率（核心包分别 ≥85%）"
	@echo "make invariant  - 业务不变量核对（名额/未到店/退款标记）"
	@echo "make smoke   - 上线冒烟（需服务已启动）"
	@echo "make race    - 并发/竞争检测"
	@echo "make vet fmt - 静态检查 / 格式化"
	@echo "make seed    - 初始化演示数据（幂等）"
	@echo "make run-all - 启动 service(9001) + gateway(8081) + paysim(9002)"

build:
	go build -o $(BIN)/service.exe ./cmd/service
	go build -o $(BIN)/gateway.exe ./cmd/gateway
	go build -o $(BIN)/paysim.exe ./cmd/paysim
	go build -o $(BIN)/seed.exe ./cmd/seed

vet:
	go vet ./...

fmt:
	gofmt -l -w .

test:
	go test ./... -count=1 -p 1

# 覆盖率（统计口径：以 ./internal/... 全包为分母，与 CI 门禁一致；门禁 ≥85%）
cover:
	-mkdir coverage
	go test ./internal/... -count=1 -p 1 -coverpkg=./internal/... -coverprofile=coverage/cover.out
	go tool cover -func=coverage/cover.out | tail -1

# 逐包覆盖率：题面要求核心业务包分别 ≥85%，只报整体会漏判
cover-pkgs:
	go run ./tools/covergate -min 85

# 业务不变量核对（读 TEST_DB_DSN 或 config.local.yaml 的 test_dsn）
invariant:
	go run ./tools/invariantcheck

# 上线冒烟（compose 部署或本机三进程启动后执行）
smoke:
	bash scripts/smoke.sh http://127.0.0.1:8081

# 测试共用一个物理库，-p 1 串行执行各包，避免并行 AutoMigrate/清库互相干扰
race:
	go test ./internal/... -race -count=1 -p 1

seed:
	go run ./cmd/seed

run:
	go run ./cmd/service

run-gateway:
	go run ./cmd/gateway

run-paysim:
	go run ./cmd/paysim

# 一次启动三个进程（Windows 下各自新窗口，便于直接用页面验收）
run-all:
	go build -o $(BIN)/service.exe ./cmd/service
	go build -o $(BIN)/gateway.exe ./cmd/gateway
	go build -o $(BIN)/paysim.exe ./cmd/paysim
	start "service" /B $(BIN)/service.exe
	start "gateway" /B $(BIN)/gateway.exe
	start "paysim" /B $(BIN)/paysim.exe

clean:
	-rm -rf $(BIN) coverage
