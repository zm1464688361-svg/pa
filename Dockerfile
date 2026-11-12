# ---- 构建阶段 ----
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGET=service
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/${TARGET}

# ---- 运行阶段 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=build /out/app /app/app
# 页面模板通过 go:embed 编译进二进制；DSN 等敏感配置一律通过环境变量注入，不打入镜像
ENV DB_DSN=""
ENTRYPOINT ["/app/app"]
