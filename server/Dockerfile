# WendaoIotPanel 镜像：同一镜像按启动命令承载两个进程角色
#   wendao-server：Go 后端（HTTP API / WebSocket / 业务 MQTT 客户端）
#   wendao-broker：独立 MQTT broker（设备接入 1883/8883，认证/ACL/互踢）
# 多阶段构建；国内网络用 goproxy.cn（可用 --build-arg GOPROXY=... 覆盖）。
#
# 构建（在 server/ 目录）： docker build -t wendao/app:latest .
# 运行：compose 分别以 entrypoint wendao-server / wendao-broker 启动两个容器；
#       纯环境变量配置（WQ_*，见 internal/config/config.go applyEnv）。

# ---- 构建阶段 ----
FROM golang:1.25-alpine AS builder

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

# 先拷 go.mod/go.sum 充分利用层缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/wendao-server ./cmd/server/ \
 && go build -trimpath -ldflags="-s -w" -o /out/wendao-broker ./cmd/broker/

# ---- 运行阶段 ----
FROM alpine:3.20

# ca-certificates: 设备 TLS 校验与外呼 https；tzdata: Asia/Shanghai；
# wget: broker/后端容器健康探针
RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S wendao && adduser -S wendao -G wendao

COPY --from=builder /out/wendao-server /usr/local/bin/wendao-server
COPY --from=builder /out/wendao-broker /usr/local/bin/wendao-broker

# broker 持久化（pebble）与 TLS 证书挂载点
RUN mkdir -p /data/broker /etc/wendao/certs && chown -R wendao:wendao /data /etc/wendao
VOLUME ["/data/broker"]

USER wendao
WORKDIR /home/wendao

ENV TZ=Asia/Shanghai

# 默认运行后端；broker 容器在 compose 中覆盖 entrypoint。
ENTRYPOINT ["wendao-server"]
