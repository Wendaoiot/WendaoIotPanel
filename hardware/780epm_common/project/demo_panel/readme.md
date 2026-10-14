# demo_panel —— wendaoiot 面板（云端平台）连接示例

Air780EPM（LuatOS）通过 4G 连接 wendaoiot 面板云平台，跑通「设备 ⇄ 平台」完整数据通道：**周期上报面板数据、接收并应答平台控制指令、上下线状态（遗嘱）**。本示例不接外设，专门用于最快验证「设备能否上平台、平台能否看到数据/下发控制」。

> 这就是 `panel_demo` 连面板（wendaoiot Panel）场景的最小可运行版本，去掉了 OTA、看门狗、多路网卡、低功耗等与「连面板」无关的部分。

## 数据流与主题

```
设备 ──每2s──► wendao/<IMEI>/data          面板数据(JSON, qos=1)
设备 ◄──────── wendao/<IMEI>/control       平台/管理员下发控制指令(JSON)
设备 ──应答──► wendao/<IMEI>/control/ack   控制结果 {"id","code":0,"msg":"ok"}
设备异常断线   wendao/<IMEI>/status         遗嘱 {"status":"offline"}
```

`data` 上报报文格式（与面板产品数据点格式一致）：

```json
{
  "id": "msg_1756700000_12",
  "ts": 1756700000,
  "version": "001.999.001",
  "first_ts": 1756700000,
  "tags": { "temperature": 26.5, "humidity": 72, "voltage": 24.6, "current": 1.12 }
}
```

`control` 下行示例（平台发布）：`{"id":"a1b2","tags":{"relay1":1}}`
设备回复 `control/ack`：`{"id":"a1b2","code":0,"msg":"ok"}`

## 文件说明

| 文件 | 作用 |
|---|---|
| `main.lua` | 程序入口：板载电源/看门狗初始化，加载模块 |
| `mqtt_wendao.lua` | wendaoiot 平台连接（通用文件，不用改） |
| `panel_app.lua` | 面板协议业务：周期上报 `data`、解析 `control` 并回 `ack`（参数在文件顶部） |

> `tags` 里 4 个量是随帧变化的**演示值**（让面板数据点能看到刷新）；接入真实项目时，把 `panel_app.lua` 里 `report_panel_data()` 的 `tags` 换成真实采集值即可（可联用 `demo_4_20ma` / `demo_0_5v` / `demo_rs485` 的采集结果）。控制动作在 `control` 处理的 `TODO` 处加继电器/DO 操作。

## 硬件接线

本示例**不需要接任何传感器/串口**，只要给 38盒子上电、插 SIM 卡、USB 连电脑烧录即可。
（GPIO37/38 供电使能、GPIO27 喂狗沿用板载电路，自研板无对应电路时按 `main.lua` 注释删除。）

## 烧录步骤

1. Luatools 用 USB 连接 38盒子，选固件 **LuatOS-SoC V2024 Air780EPM**（仓库 `core/LuatOS-SoC_V2024_Air780EPM/`，建议 `106` 版）。
2. 添加本工程目录（`demo_panel`）为脚本，下载，记录日志中的 `imei`（DEVICE IMEI）。

## 平台说明

默认连接 wendaoiot：Broker `panel.wendaoiot.com:1883`，设备身份 = IMEI（无用户名密码），主题前缀 `wendao`。
接自己平台改 `mqtt_wendao.lua` 顶部配置区 3 个常量（`HOST` / `PORT` / `TOPIC_PREFIX`）。

## 上位机验证（MQTTX）

1. MQTTX 连接 `panel.wendaoiot.com:1883`（用户名密码留空，ClientID 随便填且不要和设备相同）。
2. **看上报**：订阅 `wendao/+/data` → 应每 2 秒收到本设备的一帧 JSON，`tags` 里数值持续变化；订阅 `wendao/+/status`，设备掉线时可见 `offline`。
3. **控下发**：向 `wendao/<本设备IMEI>/control` 发布
   `{"id":"test-1","tags":{"relay1":1}}`
   → 设备日志打印 `set relay1 = 1`，并向 `wendao/<本设备IMEI>/control/ack` 回复 `{"id":"test-1","code":0,"msg":"ok"}`（MQTTX 订阅 `wendao/+/control/ack` 可见）。

## 接入真实数据

- 数据来源换成真实采集：在 `panel_app.lua` 的 `tags` 里放 ADC/串口计算出的值即可，例如：
  ```lua
  tags = { voltage_v = adc_value, current_ma = current_value, switch = dio_value }
  ```
- 数据点字段名按需与平台产品物模型保持一致；`id/ts/version/first_ts` 字段不要删（平台用它们做时序与版本管理）。
- 控制继电器：在 `for key,value in pairs(cmd.tags)` 循环里加 `gpio.setup(pin, value)` 之类的动作。

## 常见问题

- **MQTTX 收不到 data**：先看设备日志是否出现 `mqtt_wendao connected`；SIM 是否欠费、天线是否接好。
- **平台不识别设备/数据点不刷新**：确认平台侧以「IMEI」注册设备，且上报主题/字段与产品物模型一致。
- **本工程不接任何外设**，仅用于验证「设备能否连上面板平台、平台能否看到数据/能否下发控制」；需要真实采集外设请用 `demo_ttl` / `demo_rs485` / `demo_4_20ma` / `demo_0_5v`。
