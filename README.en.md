<p align="center">
  <b>English</b> &nbsp;|&nbsp; <a href="./README.md"><b>简体中文</b></a>
</p>

# WendaoIotPanel

WendaoIotPanel is an open-source general-purpose IoT system SaaS platform that provides complete capabilities for IoT device access, data collection, remote monitoring, and management. It solves the data link problem from device-side to cloud in IoT projects, enabling developers to quickly deploy their own IoT platforms.

This platform is developed by Wendaoiot and has been deployed in production environments across multiple industries, including smart water management, smart agriculture, smart aquaculture, and smart cities.

- Device-side uses standard MQTT protocol for access, without device model restrictions (device ID can be module IMEI, MAC, SN, etc.); Hezhou Air780EPM (LuatOS) is one of the supported models. Reference firmware is available in `hardware/`. Backend: Go + Frontend: Vue3/uni-app.
- Online Platform: Admin console http://panel.wendaoiot.com/login | Customer H5: http://panel.wendaoiot.com/iot | Docs wiki: http://wiki.wendaoiot.com ｜ Health Check: http://panel.wendaoiot.com/api/v1/health
- Code Repositories:
  - Gitee: https://gitee.com/wendaoiot/WendaoIotPanel
  - GitHub: https://github.com/Wendaoiot/WendaoIotPanel

**This project is licensed under the Apache 2.0 open-source license, allowing both commercial and non-commercial use.**

## Documentation

- [设备接入指南.md](./设备接入指南.md) (Chinese): device-side onboarding guide — connection params, uplink/downlink payloads, ping keepalive, OTA, D2D and error codes.
- [项目说明.md](./项目说明.md) (Chinese): architecture, multi-tenancy model, MQTT/HTTP protocol, device onboarding (per-device / per-product secret), online detection, OTA, D2D.
- [部署与测试.md](./部署与测试.md) (Chinese): local launch, production deployment, configuration reference, MQTT/TLS, automated tests.

## Requirements

| Dependency | Version | Purpose |
| --- | --- | --- |
| Go | 1.25+ | Backend build/run |
| Node.js | 18+ | Frontend build (tested on v24) |
| MySQL | 8.0+ | Business database (`wendaoiot`, utf8mb4) |
| MQTT Broker | Built-in (mochi-mqtt, MIT) | Standalone `wendao-broker` service: device auth/ACL, session kick, 8883 TLS |

> The MQTT broker is a standalone service shipped in this repository (`server/cmd/broker`,
> mochi-mqtt, MIT-licensed). It is deployed independently just like the Go backend and MySQL;
> no third-party broker is required.

Docker Desktop is optional on the dev machine: [`deploy/docker/`](./deploy/docker/) provides a ready-to-use MySQL + standalone MQTT broker + Go backend + Nginx Compose stack (secrets injected via `.env`).

## Project Structure

```
wendaoiotpanel/
├── server/                 # Go backend + standalone MQTT broker (Gin + GORM + paho MQTT + mochi-mqtt + JWT)
│   ├── cmd/
│   │   ├── server/         # Backend entry main.go (HTTP routes / WebSocket / jobs)
│   │   └── broker/         # Standalone MQTT broker entry (device access 1883/8883, auth/ACL/kick)
│   ├── internal/
│   │   ├── config/         # Config loading (config.yaml + WQ_ env vars)
│   │   ├── model/          # GORM models and AutoMigrate
│   │   ├── store/          # Data access (tenant scoping, soft/hard delete)
│   │   ├── broker/         # Broker wiring: auth / ACL / session kick (mochi-mqtt hooks)
│   │   ├── internalhttp/   # Broker internal HTTP (health/kick) and device-status reporter
│   │   ├── syncapi/        # broker↔backend internal HTTP contract (X-Sync-Key)
│   │   ├── handler/        # HTTP API
│   │   ├── mqtt/           # Backend business MQTT client, kick delegation
│   │   ├── protocol/       # Topic and payload conventions
│   │   ├── evaluate/       # Tag formula engine
│   │   ├── metrics/        # Message metrics
│   │   └── events/         # WebSocket event bus (per-tenant broadcast)
│   ├── pkg/                # crypto (bcrypt/random password), token (JWT)
│   └── config.example.yaml # Config template (copy to config.yaml)
├── web-admin/              # Vue3 + Element Plus admin panel (:3000)
├── web-app/                # uni-app (H5 + WeChat Mini Program) client (:3001)
├── hardware/               # Air780EPM (LuatOS) reference firmware & demos
│   └── 780epm_common/
│       ├── core/           # Official firmware .soc (included, needed to flash)
│       └── project/        # demo_0_5v / demo_4_20ma / demo_panel / demo_rs485 / demo_ttl / panel_demo*
├── deploy/
│   └── docker/             # Docker Compose stack (MySQL + standalone broker + backend + Nginx), secrets via .env
├── 设备接入指南.md         # Device onboarding guide (protocol & payloads)
├── 项目说明.md             # Detailed project guide (architecture & protocol)
└── 部署与测试.md           # Deployment & testing manual
```

## Quick Start

```bash
# 1) Standalone MQTT broker (device access :1883/:8883, internal HTTP :18090)
cd server && go run ./cmd/broker/

# 2) Go backend (:8080; in a second terminal, also from server/)
cd server && go run ./cmd/server/

# 3) Admin panel (:3000)
cd web-admin && npm install && npm run dev

# Client H5 (:3001/iot/)
cd web-app && npm install && npm run dev:h5
```

The recommended way is Docker Compose, which brings up MySQL + standalone broker + backend + Nginx;
see [部署与测试.md](./部署与测试.md) and [`deploy/docker/`](./deploy/docker/README.md).
Default super admin is `admin / admin123`. For full steps, production deployment, and MQTT/TLS configuration, see [部署与测试.md](./部署与测试.md).

## HTTP API Endpoints

Base path `/api/v1`. Every endpoint except the public ones requires header `Authorization: Bearer <token>`. Roles: **SA** = super_admin, **TA** = tenant_admin.

| Method | Path | Description | Role |
| --- | --- | --- | --- |
| GET | `/health` | Health check | Public |
| POST | `/login` | Login, returns JWT | Public |
| GET | `/ws` | WebSocket realtime push (token in query) | Any |
| GET/PUT | `/me/preferences` | Current account UI preferences | Any |
| GET | `/dashboard/stats` | Dashboard statistics | Any |
| GET | `/dashboard/traffic` | Message traffic series | Any |
| POST/GET | `/tenants` | Create/list tenants | SA |
| PUT/DELETE | `/tenants/:id` | Update/delete tenant (cascade) | SA |
| POST/GET | `/projects` | Create/list projects | Any |
| GET | `/projects/device-stats` | Per-project device counts | Any |
| GET | `/projects/:id/data` | Project aggregated data | Any |
| PUT | `/projects/:id`, `/projects/:id/settings` | Edit project / settings | Any |
| POST | `/projects/:id/apply-online-default` | Apply default online rule | Any |
| DELETE | `/projects/:id` | Delete project (cascade) | Any |
| GET/POST/DELETE | `/projects/:id/tags` | Project tags | Any |
| GET/POST | `/projects/:id/commands` | List/create custom control commands | Any |
| PUT/DELETE | `/commands/:id` | Update/delete custom control command | Any |
| POST/GET | `/products`, `PUT/DELETE /products/:key` | Products (per-product secret) CRUD | Any |
| POST | `/products/:key/secret/reset` | Rotate product secret (shown once) | Any |
| POST | `/devices/batch-preregister` | Batch pre-register SNs (≤500) | Any |
| POST/GET | `/devices`, `/devices/page` | Create / list / paged search devices | Any |
| GET/PUT/DELETE | `/devices/:deviceId` | Detail / update / delete (cascade) | Any |
| PUT | `/devices/:deviceId/enabled` | Enable/disable (disable kicks session) | Any |
| POST | `/devices/:deviceId/secret/reset` | Reset per-device secret | Any |
| POST | `/devices/:deviceId/reactivate` | Re-allow dynamic registration | Any |
| GET/POST/DELETE | `/devices/:deviceId/tags` | Device tag configuration | Any |
| GET | `/devices/:deviceId/data` | Device historical data | Any |
| DELETE | `/devices/:deviceId/data` | Delete device data | SA |
| POST | `/devices/:deviceId/control` | Send control command | Any |
| GET | `/devices/:deviceId/control/:msgId` | Query control command status | Any |
| GET | `/devices/:deviceId/peer/messages` | Device-to-device message trail | Any |
| GET/POST | `/peer/allows`, `DELETE /peer/allows/:id` | Cross-tenant whitelist | Any |
| GET/POST | `/firmwares`, `DELETE /firmwares/:id` | Firmware management (URL must be https) | Any |
| GET | `/firmwares/latest` | Latest firmware by device/version | Any |
| POST/GET | `/ota/tasks` | Create/list OTA tasks | Any |
| GET/DELETE | `/ota/logs` | Query/delete OTA logs (delete SA) | Any |
| GET/DELETE | `/control-logs` | Query/delete control logs (delete SA) | Any |
| GET | `/users` | User list | Any |
| PUT | `/users/password` | Change own password | Any |
| PUT/DELETE | `/users/:id/password`, `/users/:id` | Reset user password / delete user | SA |

> Unified response: `{ "code": 0, "msg": "ok", "data": ... }`; non-zero `code` means a business error. For MQTT topics, payloads, and device credentials, see [项目说明.md](./项目说明.md).

## License

This project is licensed under the [Apache 2.0](LICENSE) open-source license.

**Allows both commercial and non-commercial use.**

**License**:
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## Contact Us

Add WeChat friend to join the WeChat group (Note: WendaoIotPanel):


![WeChat QR Code](./微信加好友.png)
