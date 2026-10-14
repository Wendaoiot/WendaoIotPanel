# 客户样例工程索引（Air780EPM / LuatOS）

> wendaoiot 平台设备侧为标准 MQTT 协议（主题 `wendao/{设备ID}/...`，设备ID 可为 IMEI/MAC/SN 等），不限定设备型号；本目录是合宙 Air780EPM（LuatOS）这一型号的参考固件。

本目录下 `demo_*` 是面向客户的**最小独立样例工程**，每个工程只演示一种采集 → MQTT 上报（wendaoiot 平台），文件最少、可直接烧录验证。

| 工程目录 | 演示内容 | 业务文件 | 上报主题 |
|---|---|---|---|
| `demo_panel/` | **连 wendaoiot 面板平台**：周期上报面板数据、接收并应答控制指令 | `panel_app.lua` | `data` 上报、`control`/`control/ack`、`status` 遗嘱 |
| `demo_ttl/` | TTL 串口（UART1, 115200）双向透传 | `uart_tl.lua` | `wendao/<IMEI>/uart/up`、下行 `uart/down` |
| `demo_rs485/` | RS485（UART2, 9600，GPIO33 硬件方向）ASCII 收发 | `rs485_app.lua` | `wendao/<IMEI>/rs485/up`、下行 `rs485/down` |
| `demo_4_20ma/` | 4-20mA 电流环（ADC1/PIN96，采样电阻 250Ω） | `adc_4_20ma.lua` | `wendao/<IMEI>/data`（`current_ma`、`percent`） |
| `demo_0_5v/` | 0-5V 电压（ADC2/PIN77，150k/53k 分压） | `adc_0_5v.lua` | `wendao/<IMEI>/data`（`voltage_v`） |

## 每个工程的文件构成（3 个 lua + 1 个说明）

```
demo_xxx/
├── main.lua         入口：PROJECT/VERSION、板载供电(GPIO37/38)与硬件看门狗(GPIO27)初始化、加载模块
├── mqtt_wendao.lua  wendaoiot 平台连接（各工程共用同一文件；换平台只改顶部 3 个常量）
├── <业务>.lua       该样例唯一的采集/透传逻辑（参数集中在文件顶部配置区）
└── readme.md        接线、烧录、MQTTX 验证步骤、常见问题
```

## 公共说明

- **固件**：LuatOS-SoC V2024 Air780EPM（见仓库 `core/LuatOS-SoC_V2024_Air780EPM/`，建议最高版本 `106`），Luatools 烧录时固件 + 工程目录脚本一起下载。
- **平台**：默认连 wendaoiot（Broker `panel.wendaoiot.com:1883`），Air780EPM 固件设备身份取模组 IMEI，主题规则 `wendao/<IMEI>/<功能>`；接自己平台改 `mqtt_wendao.lua` 顶部 `HOST` / `PORT` / `TOPIC_PREFIX`。
- **验证工具**：MQTTX 订阅 `wendao/+/data`（或 `wendao/+/uart/up` 等）即可看到本设备上报；具体步骤见各工程 `readme.md`。
- **板载依赖**：GPIO37/38 使能供电（RS485 必需）、GPIO27 喂硬件看门狗（不喂会复位）；自研板无对应电路时，按 `main.lua` 内注释删除相应段落即可。

## 历史工程（内部模板，非客户样例）

`panel_demo/`、`panel_demo_no_sleep/`、`panel_demo_sleep/`、`panel_demo_tcp_shortsleep/`
为大而全的内部开发模板（含 OTA、多网络驱动、看门狗、低功耗/PSM、多采集模块等），客户样例由其精简而来。
