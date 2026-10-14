<p align="center">
  <a href="./README.en.md"><b>English</b></a>  |  <b>简体中文</b>
</p>

# WendaoIotPanel

WendaoIotPanel 是一个开源的通用物联网系统 SAAS 平台，提供完整的物联网设备接入、数据采集、远程监控和管理能力。它解决了物联网项目从设备端到云端的数据链路问题，让开发者能够快速部署属于自己的物联网平台。

这个平台由 Wendaoiot（闻道物联）研发，已被用于智慧水务、智慧农业、智慧养殖、智慧城市等多个行业的生产环境。

- 设备端走标准 MQTT 协议接入；合宙 Air780EPM（LuatOS）是已适配型号之一，参考固件见 `hardware/`。后端 Go + 前端 Vue3/uni-app。
- 线上平台：管理后台 [http://panel.wendaoiot.com/login](http://panel.wendaoiot.com/login) ｜ C 端 H5 [http://panel.wendaoiot.com/iot](http://panel.wendaoiot.com/iot) ｜ 资料站 [http://wiki.wendaoiot.com](http://wiki.wendaoiot.com)
- 代码仓库：
  - Gitee：[https://gitee.com/wendaoiot/WendaoIotPanel](https://gitee.com/wendaoiot/WendaoIotPanel)
  - GitHub：[https://github.com/Wendaoiot/WendaoIotPanel](https://github.com/Wendaoiot/WendaoIotPanel)

**本项目采用 Apache 2.0 开源协议，允许任何商业和非商业使用。**

## 文档

- [设备接入指南.md](./设备接入指南.md)：面向端侧开发者——连接参数、上行/下行报文、ping 应答、OTA、D2D 与错误码。
- [项目说明.md](./项目说明.md)：系统架构、多租户模型、MQTT/HTTP 协议、设备接入（一机一密 / 一型一密）、在线判定、OTA、D2D。
- [部署与测试.md](./部署与测试.md)：本地启动、生产部署、配置项、MQTT/TLS、自动化测试。

## 环境依赖

| 依赖 | 版本 | 用途 |
| --- | --- | --- |
| Go | 1.25+ | 后端编译运行 |
| Node.js | 18+ | 前端构建（实测 v24） |
| MySQL | 8.0+ | 业务数据库（库名 `wendaoiot`，utf8mb4） |
| MQTT Broker | 内置（mochi-mqtt，MIT） | 独立 `wendao-broker` 服务，设备接入认证/ACL/互踢、8883 TLS |

> MQTT broker 是本仓库自带的独立服务（`server/cmd/broker`，mochi-mqtt，MIT 许可），
> 与 Go 后端、MySQL 一样独立部署，无需安装任何第三方 broker。

开发机可选 Docker Desktop：[`deploy/docker/`](./deploy/docker/) 提供开箱即用的 MySQL + 独立 MQTT broker + Go 后端 + Nginx Compose 栈（密钥经 `.env` 注入）。

## 项目结构

```
wendaoiotpanel/
├── server/                 # Go 后端 + 独立 MQTT broker（Gin + GORM + paho MQTT + mochi-mqtt + JWT）
│   ├── cmd/
│   │   ├── server/         # 后端入口 main.go（HTTP 路由 / WebSocket / 定时任务）
│   │   └── broker/         # 独立 MQTT broker 入口（设备接入 1883/8883、认证/ACL/互踢）
│   ├── internal/
│   │   ├── config/         # 配置加载（config.yaml + WQ_ 环境变量）
│   │   ├── model/          # GORM 模型与 AutoMigrate
│   │   ├── store/          # 数据访问层（多租户作用域、软/物理删除）
│   │   ├── broker/         # broker 装配：认证 / ACL / 会话互踢（mochi-mqtt hook）
│   │   ├── internalhttp/   # broker 内部 HTTP（健康/踢线）与设备状态上报
│   │   ├── syncapi/        # broker↔后端内部 HTTP 契约（X-Sync-Key）
│   │   ├── handler/        # HTTP 接口
│   │   ├── mqtt/           # 后端业务 MQTT 客户端、互踢委托
│   │   ├── protocol/       # Topic 与报文约定
│   │   ├── evaluate/       # 标签公式引擎
│   │   ├── metrics/        # 消息指标
│   │   ├── events/         # WebSocket 事件总线（按租户广播）
│   ├── pkg/                # crypto（bcrypt/随机密码）、token（JWT）
│   └── config.example.yaml # 配置模板（复制为 config.yaml）
├── web-admin/              # Vue3 + Element Plus 管理后台（:3000）
├── web-app/                # uni-app（H5 + 微信小程序）C 端（:3001）
├── hardware/               # Air780EPM（LuatOS）参考固件与客户 demo
│   └── 780epm_common/
│       ├── core/           # 合宙官方固件 .soc（已入库，烧录必需）
│       └── project/        # demo_0_5v / demo_4_20ma / demo_panel / demo_rs485 / demo_ttl / panel_demo 系列
├── deploy/
│   └── docker/             # Docker Compose 栈（MySQL + 独立 broker + 后端 + Nginx），密钥经 .env 注入
├── 项目说明.md             # 详细项目说明（架构与协议）
└── 部署与测试.md           # 部署与测试手册
```

## 本地快速启动

需先用 `server/config.example.yaml` 准备 `server/config.yaml`（或用 `WQ_*` 环境变量，本地可设 `WQ_INSECURE=1` 跳过密钥强度校验），并准备好 MySQL 8.0。

```bash
# 1) 独立 MQTT broker（设备接入 :1883/:8883，内部 HTTP :18090）
cd server && go run ./cmd/broker/

# 2) Go 后端（默认 :8080，另开一个终端，同样在 server/ 目录）
cd server && go run ./cmd/server/

# 3) 管理后台（:3000）
cd web-admin && npm install && npm run dev

# C 端 H5（:3001/iot/）
cd web-app && npm install && npm run dev:h5
```

推荐用 Docker Compose 一键拉起 MySQL + 独立 broker + 后端 + Nginx，见 [部署与测试.md](./部署与测试.md) 与 [`deploy/docker/`](./deploy/docker/README.md)。
默认超管 `admin / admin123`。完整步骤、生产部署、MQTT/TLS 配置见 [部署与测试.md](./部署与测试.md)。

## HTTP API 接口列表

基址 `/api/v1`。除「公开」接口外均需请求头 `Authorization: Bearer <token>`。角色：**超管** = super_admin，**租户** = tenant_admin。

| 方法 | 路径 | 说明 | 权限 |
| --- | --- | --- | --- |
| GET | `/health` | 健康检查 | 公开 |
| POST | `/login` | 登录，返回 JWT | 公开 |
| GET | `/ws` | WebSocket 实时推送（query 带 token） | 登录 |
| GET/PUT | `/me/preferences` | 当前账号 UI 偏好 | 登录 |
| GET | `/dashboard/stats` | 仪表盘统计 | 登录 |
| GET | `/dashboard/traffic` | 消息流量曲线 | 登录 |
| POST/GET | `/tenants` | 新建/列出租户 | 超管 |
| PUT/DELETE | `/tenants/:id` | 修改/删除租户（级联） | 超管 |
| POST/GET | `/projects` | 新建/列出项目 | 登录 |
| GET | `/projects/device-stats` | 各项目设备数量角标 | 登录 |
| GET | `/projects/:id/data` | 项目聚合数据 | 登录 |
| PUT | `/projects/:id` `/projects/:id/settings` | 编辑项目 / 项目设置 | 登录 |
| POST | `/projects/:id/apply-online-default` | 套用在线判定默认值 | 登录 |
| DELETE | `/projects/:id` | 删除项目（级联） | 登录 |
| GET/POST/DELETE | `/projects/:id/tags` | 项目标签（列表/新增/删除） | 登录 |
| GET/POST | `/projects/:id/commands` | 项目自定义控制命令列表/新增 | 登录 |
| PUT/DELETE | `/commands/:id` | 修改/删除自定义控制命令 | 登录 |
| POST/GET | `/products`、`PUT/DELETE /products/:key` | 产品（一型一密）CRUD | 登录 |
| POST | `/products/:key/secret/reset` | 轮换产品密钥（明文仅一次） | 登录 |
| POST | `/devices/batch-preregister` | 批量预录 SN（≤500） | 登录 |
| POST/GET | `/devices`、`/devices/page` | 新建设备 / 全量 / 分页检索 | 登录 |
| GET/PUT/DELETE | `/devices/:deviceId` | 设备详情 / 编辑 / 删除（级联） | 登录 |
| PUT | `/devices/:deviceId/enabled` | 启用/禁用（禁用即踢线） | 登录 |
| POST | `/devices/:deviceId/secret/reset` | 重置一机一密密钥 | 登录 |
| POST | `/devices/:deviceId/reactivate` | 产品设备重新允许动态注册 | 登录 |
| GET/POST/DELETE | `/devices/:deviceId/tags` | 设备标签配置 | 登录 |
| GET | `/devices/:deviceId/data` | 设备历史数据 | 登录 |
| DELETE | `/devices/:deviceId/data` | 删除设备数据 | 超管 |
| POST | `/devices/:deviceId/control` | 下发控制指令 | 登录 |
| GET | `/devices/:deviceId/control/:msgId` | 查询控制指令状态 | 登录 |
| GET | `/devices/:deviceId/peer/messages` | 设备间通信留痕 | 登录 |
| GET/POST | `/peer/allows`、`DELETE /peer/allows/:id` | 跨租户白名单授权 | 登录 |
| GET/POST | `/firmwares`、`DELETE /firmwares/:id` | 固件管理（URL 必须 https） | 登录 |
| GET | `/firmwares/latest` | 按设备/版本取最新固件 | 登录 |
| POST/GET | `/ota/tasks` | 创建/列出 OTA 任务 | 登录 |
| GET/DELETE | `/ota/logs` | 查询/删除 OTA 日志（删仅超管） | 登录 |
| GET/DELETE | `/control-logs` | 查询/删除控制日志（删仅超管） | 登录 |
| GET | `/users` | 用户列表 | 登录 |
| PUT | `/users/password` | 修改自身密码 | 登录 |
| PUT/DELETE | `/users/:id/password`、`/users/:id` | 重置用户密码 / 删除用户 | 超管 |

> 统一响应 `{ "code": 0, "msg": "ok", "data": ... }`，`code != 0` 为业务失败。MQTT Topic 与报文、设备接入凭据见 [项目说明.md](./项目说明.md)。

## 开源协议

本项目采用 [Apache 2.0](LICENSE) 开源协议。

**允许任何商业和非商业使用。**

## 联系我们

加好友进微信群（备注 WendaoIotPanel）：

![微信加好友](./微信加好友.png)
