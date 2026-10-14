# demo_4_20ma —— 4-20mA 电流环采集上报示例

Air780EPM（LuatOS）通过 4G 连接 wendaoiot 平台，采集 **4-20mA 电流环**信号，换算成电流值（mA）和量程百分比（%），每 2 秒一次以 JSON 上报。适合工业变送器（压力、液位、温度、流量等两线制传感器）。

## 功能与数据流

```
两线制变送器/信号源 ──4~20mA──► 4~20mA_INPUT
        R18(250Ω)→GND：I×R 得电压（4mA→1V，20mA→5V）
        150k/53k 分压（GPIO20 拉低使能）→ 5V→1.305V
        ADC1（模组 PIN96）──换算──► MQTT wendao/<IMEI>/data
```

上报 payload 示例：

```json
{"id":"msg_1756700000_4821","ts":1756700000,"tags":{"current_ma":12.01,"percent":50.1}}
```

- `current_ma`：实测电流（mA）；`percent`：量程百分比（4mA=0%，20mA=100%）
- 电流低于约 3.5mA 时上报 `"fault":"loop_broken"` 且 `percent=-1`，表示回路断线/未接（断线时电流为 0）

## 文件说明

| 文件 | 作用 |
|---|---|
| `main.lua` | 程序入口：板载电源/看门狗初始化，加载模块 |
| `mqtt_wendao.lua` | wendaoiot 平台连接（通用文件，不用改） |
| `adc_4_20ma.lua` | 采集与换算业务（采样电阻、系数、周期都在文件顶部配置区） |

## 硬件接线（38盒子 V3.1）

| 信号 | 端子/引脚 | 说明 |
|---|---|---|
| 4~20mA_INPUT | 面板 `4~20mA输入` 端子 | 变送器电流回路串入/信号源正极输入 |
| GND | GND | 变送器/信号源与盒子共地 |

板载电路（对应原理图）：输入电流经 **R18 = 250Ω** 采样电阻到地转成电压，再经 **R20(150k)/R21(53k)** 分压网络（下端由 **GPIO20 拉低** 使能）进 **ADC1（模组 PIN96）**。

> ⚠️ 若您的板子 R18 实际阻值不同，把 `adc_4_20ma.lua` 顶部的 `R18_OHM` 改为实际值。

## 烧录步骤

1. Luatools 用 USB 连接 38盒子，选固件 **LuatOS-SoC V2024 Air780EPM**（仓库 `core/LuatOS-SoC_V2024_Air780EPM/`，建议 `106` 版）。
2. 添加本工程目录（`demo_4_20ma`）为脚本，下载，记录日志中的 `IMEI`。

## 平台说明

默认 Broker `panel.wendaoiot.com:1883`，数据主题 `wendao/<IMEI>/data`，设备身份 = IMEI。
接自己平台改 `mqtt_wendao.lua` 顶部配置区 3 个常量（`HOST` / `PORT` / `TOPIC_PREFIX`）。

## 上位机验证（MQTTX + 信号发生器/变送器）

1. MQTTX 连接 `panel.wendaoiot.com:1883`，订阅 `wendao/+/data`。
2. 用 4-20mA 信号发生器在 `4~20mA输入` 端子输入电流（或直接调变送器输出）：

| 输入电流 | `current_ma` 应约为 | `percent` 应约为 |
|---|---|---|
| 4 mA | 4.0 | 0 |
| 12 mA | 12.0 | 50 |
| 20 mA | 20.0 | 100 |
| 0 mA（断线） | ≈0 | -1，且带 `fault:loop_broken` |

3. 数值偏差稍大时做两点标定：输入 4mA、20mA，微调 `adc_4_20ma.lua` 顶部的 `OFFSET_MV`（整体零点平移）和 `R18_OHM`（增益）。

## 换算成实际物理量（温度/压力/液位）

变送器量程已知时（例：0~1.6MPa），在 `adc_4_20ma.lua` 的上报里再加一个字段即可：

```lua
local pressure_mpa = percent / 100 * 1.6      -- 0~100% 线性映射到 0~1.6MPa
mqtt_wendao.report({ current_ma = current_ma, percent = percent, pressure = pressure_mpa })
```

## 常见问题

- **上报电流一直为 0 且报 loop_broken**：回路未接通/断线；确认两线制回路方向与共地。
- **数值整体偏小/偏大**：先调 `OFFSET_MV`（默认 +200mV）；增益偏差精调 `R18_OHM`。
- **用第二路模拟量**：0-5V 电压请参考相邻的 `demo_0_5v` 工程（ADC2/PIN77/GPIO4）；板载另一路 0-5V 是 ADC3（PIN76，使能 GPIO28），改法相同。
