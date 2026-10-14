# deploy/docker：容器化生产栈

> MQTT broker（mochi-mqtt，MIT）是本仓库自带的**独立服务** `wendao-broker`，
> 与 MySQL、Go 后端、Nginx 同为独立容器，无需安装任何第三方 broker。
> 设计与运维详见 [docs/容器化部署运维手册.md](../../docs/容器化部署运维手册.md)。

## 文件

| 文件 | 说明 |
| --- | --- |
| `docker-compose.yml` | 四服务：broker / wendao（后端）/ mysql / nginx |
| `.env.example` | 环境变量模板（复制为 `.env`；`.env` 不入库） |
| `gen-secrets.sh` | 生成强随机密钥版 `.env`（已存在则拒绝覆盖） |
| `gen-certs.sh` | 生成 8883 自签 CA 与服务器证书（`FORCE_CERTS=1` 轮换） |
| `Dockerfile.portal` | 门户镜像：构建 web-admin + nginx 服务 + wiki 静态站 |
| `nginx/panel.conf` | panel / wiki 双 server 站点配置 |
| `../../server/Dockerfile` | 应用镜像：golang:1.25-alpine 多阶段构建出 `wendao-server` 与 `wendao-broker` 两个二进制 |

## 服务拓扑

| 服务 | 容器名 | 角色 | 宿主端口 |
| --- | --- | --- | --- |
| `broker` | wq-broker | 独立 MQTT broker：设备接入、认证/ACL/互踢；认证只读 MySQL，上下线/踢线经内部 HTTP 与后端通信 | 1883、8883 |
| `wendao` | wq-wendao | Go 后端：HTTP API / WebSocket / 业务 MQTT 客户端 / 定时任务 | 不暴露（仅网内） |
| `mysql` | wq-mysql | MySQL 8.0，命名卷持久化 | 不暴露 |
| `nginx` | wq-nginx | web-admin + wiki 静态站 + `/api` 反代 | 8088（默认） |

后端等待 broker 健康后启动；二者经 compose 内网 `wendao-net` 通信，
内部 HTTP（broker `:18090`、后端 `:8080/internal`）不映射宿主，以 `X-Sync-Key` 共享密钥鉴权。

## 快速开始

```bash
# 1) 生成 .env（强随机密钥）或 cp .env.example .env 手工修改
./gen-secrets.sh

# 2) 生成 8883 TLS 证书（设备侧需导入 certs/ca.crt）
./gen-certs.sh

# 3) 构建并启动
docker compose build
docker compose up -d
docker compose ps        # 全部 healthy/running
```

## 端口

| 端口 | 服务 | 说明 |
| --- | --- | --- |
| 8088 | nginx | web-admin + wiki + `/api` 反代（宿主 80 被占用时默认 8088，再由宿主 nginx 反代） |
| 1883 | broker | MQTT 明文（设备接入） |
| 8883 | broker | MQTT over TLS（设备接入，需导入 ca.crt） |
| 8080 | wendao | HTTP API，仅容器网内，经 nginx 反代访问 |
| 18090 | broker | 内部 HTTP（健康探针/踢线），仅容器网内 |

> 腾讯云安全组只需放行 `80/TCP`（或宿主反代端口）、`1883/TCP`、`8883/TCP`。

## 首次登录

`gen-secrets.sh` 输出管理员初始密码（也在 `.env` 的 `WQ_ADMIN_PASSWORD`）。
打开 `http://panel.wendaoiot.com/` → 302 到 `/login` → 用 `admin / <初始密码>` 登录。

## 常用命令

```bash
docker compose ps                    # 状态
docker compose logs -f broker        # broker（设备接入/认证/互踢）日志
docker compose logs -f wendao        # 后端日志
docker compose restart broker        # 重启 broker（证书轮换后）
docker compose restart wendao        # 重启后端
docker compose down                  # 停止（保留数据卷）
docker compose down -v               # 停止并清空 MySQL + broker 会话（慎用）
```

## 安全提醒

- `.env`、`certs/`（含 CA 与服务器私钥）不入库（根 `.gitignore` 已覆盖）。
- 管理员初始密码首次登录后尽快修改。
- 认证、ACL、互踢全部在 `wendao-broker` 进程内完成，无第三方 broker 的 Dashboard/API Key/HTTP 回调需要维护；
  broker↔后端内部 HTTP 仅内网可达且以 `WQ_SYNC_SECRET` 共享密钥鉴权。
