# demo_ttl —— TTL 串口数据采集上报示例

Air780EPM（LuatOS）通过 4G 连接 wendaoiot 平台，把 **TTL 串口收到的数据透传到 MQTT 平台**，平台下发的数据也会从串口原样输出。适合串口设备（仪表、传感器、单片机）的数据远程采集。

## 功能与数据流

```
串口设备/USB转TTL  ──TX/RX(TTL,115200)──►  Air780EPM UART1  ──MQTT──►  wendaoiot 平台
                     平台下发 wendao/<IMEI>/uart/down  ◄── MQTT ──  平台
```

- 串口收到一帧数据（50ms 空闲判包）→ 发布到 `wendao/<IMEI>/uart/up`（QoS 1）
- 平台发布到 `wendao/<IMEI>/uart/down` 的内容 → UART1 原样输出
- 断网自动重连；`<IMEI>` 是模组 15 位串号，设备上电日志会打印

## 文件说明

| 文件 | 作用 |
|---|---|
| `main.lua` | 程序入口：板载电源/看门狗初始化，加载模块 |
| `mqtt_wendao.lua` | wendaoiot 平台连接（4 个示例共用的通用文件，不用改） |
| `uart_tl.lua` | TTL 串口透传业务（串口参数、主题后缀都在本文件顶部） |

## 硬件接线（38盒子 V3.1）

| 38盒子 TTL 口 | USB转TTL | 说明 |
|---|---|---|
| GND | GND | **必须共地** |
| TXD（模组 UART1_TX） | RXD | 模组发 → USB 转串口收 |
| RXD（模组 UART1_RX） | TXD | USB 转串口才发 → 模组收（注意交叉） |

串行参数：**115200 波特率、8 数据位、1 停止位、无校验**。

> 想用板载另一路 TTL（UART3，模组 PIN39=RX/PIN40=TX）：把 `uart_tl.lua` 里
> `local UART_ID = 1` 改为 `3`，并在模组配置里关闭 LCD（UART3 与 LCD 复用引脚）。

## 烧录步骤

1. 打开 Luatools，用 USB 线连接 38盒子（USB_BOOT 可强制进烧录模式）。
2. 选择固件 **LuatOS-SoC V2024 Air780EPM**（仓库 `core/LuatOS-SoC_V2024_Air780EPM/`，建议用最高版本 `106`）。
3. 添加本工程目录（`demo_ttl`）作为脚本文件。
4. 下载。上电后 Luatools 日志可见 `DEVICE IMEI: 86XXXXXXXXXXXXX`，记录该 IMEI。

## 平台说明

默认连接 wendaoiot 平台：

- Broker：`panel.wendaoiot.com:1883`（明文 TCP）
- 主题规则：`wendao/<IMEI>/<功能>`
- 设备身份 = IMEI（无需用户名密码）

**接自己的平台**：改 `mqtt_wendao.lua` 顶部配置区 3 个常量：

```lua
local HOST = "你的服务器IP或域名"
local PORT = 1883
local TOPIC_PREFIX = "你的主题前缀"
```

## 上位机验证（MQTTX + 串口助手）

1. 串口助手选 USB转TTL 串口，115200 8N1 打开。
2. MQTTX 连接 `panel.wendaoiot.com:1883`，用户名密码留空，订阅：
   `wendao/+/uart/up`（`+` 通配，能看到所有设备上行）。
3. **验上行**：串口助手发送任意字符串 → MQTTX 立刻收到该字符串（主题 `wendao/<本设备IMEI>/uart/up`）。
4. **验下行**：MQTTX 向 `wendao/<本设备IMEI>/uart/down` 发布任意字符串 → 串口助手立刻显示。

## 常见问题

- **串口助手发了数据但 MQTTX 收不到**：先确认日志里 MQTT 已 `connected`；检查 TX/RX 是否接反、是否共地、波特率是否一致。
- **设备反复重启**：38盒子有硬件看门狗（GPIO27 喂狗），`main.lua` 里喂狗任务不可删；自己做的板子无看门狗电路时可删掉该段。
- **中文/二进制数据**：透传不解析内容，任意字节均可；平台显示乱码是查看器编码问题，不影响数据。
