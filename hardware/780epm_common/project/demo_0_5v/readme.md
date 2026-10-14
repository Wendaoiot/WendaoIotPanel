# demo_0_5v —— 0-5V 电压采集上报示例

Air780EPM（LuatOS）通过 4G 连接 wendaoiot 平台，采集 **0~5V 电压输入**，换算成电压值（V），每 2 秒一次以 JSON 上报。适合 0-5V 输出的传感器（电压型变送器、电位器、电池电压监测等）。

## 功能与数据流

```
可调电源/0-5V传感器 ──► 0~5V_INPUT1
        150k/53k 分压（GPIO4 拉低使能）→ 5V→1.305V，电容 C24 滤波
        ADC2（模组 PIN77）──换算──► MQTT wendao/<IMEI>/data
```

上报 payload 示例：

```json
{"id":"msg_1756700000_4821","ts":1756700000,"tags":{"voltage_v":2.497}}
```

## 文件说明

| 文件 | 作用 |
|---|---|
| `main.lua` | 程序入口：板载电源/看门狗初始化，加载模块 |
| `mqtt_wendao.lua` | wendaoiot 平台连接（通用文件，不用改） |
| `adc_0_5v.lua` | 采集与换算业务（通道、使能脚、系数、周期都在文件顶部配置区） |

## 硬件接线（38盒子 V3.1）

| 信号 | 端子/引脚 | 说明 |
|---|---|---|
| 0~5V_INPUT1 | 面板 `0~5V输入1` 端子 | 电压信号正极（0~5V，**请勿超过 6V**） |
| GND | GND | 信号源与盒子共地 |

板载电路（对应原理图）：输入电压经 **R39(150k)/R40(53k)** 分压（下端由 **GPIO4 拉低** 使能）、**C24** 滤波后进 **ADC2（模组 PIN77）**。分压还原系数 203/53 ≈ 3.8302，板级经验校正 +200mV。

> 第二路 0-5V（端子 `0~5V输入2`）：把 `adc_0_5v.lua` 配置区两行改为
> `local ADC_ID = 3`（ADC3，模组 PIN76）、`local DIV_GPIO = 28`（GPIO28），文件末尾有注释说明。

## 烧录步骤

1. Luatools 用 USB 连接 38盒子，选固件 **LuatOS-SoC V2024 Air780EPM**（仓库 `core/LuatOS-SoC_V2024_Air780EPM/`，建议 `106` 版）。
2. 添加本工程目录（`demo_0_5v`）为脚本，下载，记录日志中的 `IMEI`。

## 平台说明

默认 Broker `panel.wendaoiot.com:1883`，数据主题 `wendao/<IMEI>/data`，设备身份 = IMEI。
接自己平台改 `mqtt_wendao.lua` 顶部配置区 3 个常量（`HOST` / `PORT` / `TOPIC_PREFIX`）。

## 上位机验证（MQTTX + 可调电源）

1. MQTTX 连接 `panel.wendaoiot.com:1883`，订阅 `wendao/+/data`。
2. 可调电源（共地）给 `0~5V输入1` 加电压：

| 输入电压 | `voltage_v` 应约为 |
|---|---|
| 0 V | 0.0 |
| 2.5 V | 2.5 |
| 5.0 V | 5.0 |

3. 有偏差时两点标定：输入 0V 和 5.000V，微调 `adc_0_5v.lua` 顶部 `OFFSET_MV`（零点）；偏差呈比例时微调 `EXT_DIV_SCALE`。

## 换算成实际物理量

传感器量程已知时（例：0-5V 对应 0-100°C），在上报处加一个字段：

```lua
local temperature_c = voltage_v / 5 * 100      -- 0~5V 线性映射到 0~100°C
mqtt_wendao.report({ voltage_v = voltage_v, temperature = temperature_c })
```

## 常见问题

- **读数为 0 不随输入变化**：确认 GPIO4 已拉低使能（用本工程即已配置）；输入是否接到 `0~5V输入1`（不是 4-20mA 端子）；是否共地。
- **数值整体偏小/偏大**：先调 `OFFSET_MV`（默认 +200mV）；比例偏差调 `EXT_DIV_SCALE`。
- **要采集 4-20mA 电流信号**：请用相邻的 `demo_4_20ma` 工程（ADC1/PIN96/GPIO20，端子 `4~20mA输入`）。
