# 宠物到店服务预约系统（题目 02）— 构建/测试/运行
# Windows 本机 + GitHub Actions 均可执行。

BIN   := bin

# 数据库 DSN：优先环境变量 DB_DSN，其次 config.local.yaml（不入库）
.PHONY: help build test cover race vet fmt seed run run-all clean smoke

help:
	@echo "make build   - 编译 service/gateway/paysim/seed 到 bin/"
	@echo "make test    - 全部单元测试"
	@echo "make cover   - 覆盖率（门禁：核心包 ≥85%）"
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
	go test ./... -count=1

# 覆盖率（统计口径：以 ./internal/... 全包为分母，与 CI 门禁一致；门禁 ≥85%）
cover:
	go test ./internal/... -count=1 -coverpkg=./internal/... -coverprofile=coverage/cover.out
	go tool cover -func=coverage/cover.out | tail -1

race:
	go test ./internal/... -race -count=1

seed:
	go run ./cmd/seed

run:
	go run ./cmd/service

run-gateway:
	go run ./cmd/gateway

run-paysim:
	go run ./cmd/paysim

clean:
	rm -rf $(BIN) coverage
