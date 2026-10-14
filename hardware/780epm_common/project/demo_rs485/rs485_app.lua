--[[
@module  rs485_app
@summary RS485(UART2 + GPIO33 方向控制) 收发与 wendaoiot 平台互通
@version 1.0

@usage
数据流：
  平台 → wendao/<IMEI>/rs485/down → UART2_TX →(RS485)→ 总线设备
  总线设备 →(RS485)→ UART2_RX → wendao/<IMEI>/rs485/up   (qos=1)
  另外每 2 秒主动发送一帧 ASCII 测试串，用于验证本设备发送方向是否正常。

硬件（38盒子 V3.1）：
  UART2_TX/RX 接 RS485 收发器 DI/RO，收发器 DE/nRE 接 GPIO33（485_DIR）；
  RS485_5V 由 EN1(GPIO37) 使能（已在 main.lua 拉高）；
  对外端子 485-A、485-B 接总线（A 接 A、B 接 B）。

串行参数：9600 波特率、8 数据位、1 停止位、无校验。
注意：本示例只做透明 ASCII 收发验证，不是 Modbus 协议。
]]

local mqtt_wendao = require "mqtt_wendao"

-- 串口参数
local UART_ID = 2            -- RS485 使用 UART2
local BAUD = 9600            -- 9600 8N1
-- RS485 方向控制脚：收发器 DE、/RE 共同接此脚
local RS485_DIR_PIN = 33     -- GPIO33 = 485_DIR
local RS485_RX_DIR_LEVEL = 0 -- 接收时 DIR 电平 = 0（硬件高电平=发送 → 接收为低）
local RS485_DIR_DELAY = 2000 -- 发送完成后保持发送方向 2ms(us) 再切回接收
local SEND_INTERVAL_MS = 2000 -- 测试串发送周期
-- 主题后缀（完整主题 = wendao/<IMEI>/<后缀>）
local UP_SUFFIX = "rs485/up"
local DN_SUFFIX = "rs485/down"

local rpt_buf = ""          -- 上报拼包缓冲
local send_count = 0

-- 50ms 空闲判包后整体上报
local function concat_timeout_func()
    if #rpt_buf > 0 then
        mqtt_wendao.pub(UP_SUFFIX, rpt_buf, 1)
        rpt_buf = ""
    end
end

-- UART2 接收回调：收到 485 总线数据
local function uart_receive_func(id)
    while true do
        local s = uart.read(id, 1024)
        if not s or #s == 0 then
            -- 50ms 空闲判包
            sys.timerStart(concat_timeout_func, 50)
            break
        end
        log.info("rs485_app.recv", #s, "bytes")
        rpt_buf = rpt_buf .. s
    end
end

-- 初始化 UART2（485 模式）
-- uart.setup 第 8/9/10 个参数启用硬件自动方向控制：
--   方向脚 GPIO33、接收时方向脚电平 0、发完保持 2ms；uart.write 时固件自动拉高发完拉低
uart.setup(UART_ID, BAUD, 8, 1, uart.NONE, uart.LSB, 1024,
           RS485_DIR_PIN, RS485_RX_DIR_LEVEL, RS485_DIR_DELAY)
uart.on(UART_ID, "receive", uart_receive_func)

-- 订阅平台下行主题，收到后通过 485 发到总线
mqtt_wendao.subscribe(DN_SUFFIX)
sys.subscribe("WENDAO_RECV", function(suffix, payload)
    if suffix == DN_SUFFIX then
        uart.write(UART_ID, payload)
    end
end)

-- 周期发送测试串任务：每 2 秒一帧，验证本设备发送通路
sys.taskInit(function()
    sys.wait(2000)   -- 等 RS485 收发器上电稳定
    while true do
        send_count = send_count + 1
        local test_data = string.format("rs485 test #%d, imei=%s\r\n", send_count, mobile.imei())
        -- uart.write 期间固件自动控制 GPIO33 方向（发送拉高，完成拉低）
        uart.write(UART_ID, test_data)
        sys.wait(SEND_INTERVAL_MS)
    end
end)

log.info("rs485_app", "setup done: uart" .. UART_ID, BAUD .. " 8N1, dir=GPIO" .. RS485_DIR_PIN)
log.info("rs485_app", "up topic:   wendao/<IMEI>/" .. UP_SUFFIX)
log.info("rs485_app", "down topic: wendao/<IMEI>/" .. DN_SUFFIX)
