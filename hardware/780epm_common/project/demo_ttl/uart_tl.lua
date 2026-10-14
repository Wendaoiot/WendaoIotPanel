--[[
@module  uart_tl
@summary TTL 串口(UART1) 与 wendaoiot 平台双向透传
@version 1.0

@usage
数据流：
  外设/串口助手 →(TTL)→ UART1_RX → wendao/<IMEI>/uart/up   (qos=1)
  平台 → wendao/<IMEI>/uart/down → UART1_TX →(TTL)→ 外设/串口助手

硬件：UART1 使用模组默认引脚（38盒子 V3.1 板载 TTL 串口座），
      USB转TTL 模块共地后，模组 TXD 接模块 RXD、模组 RXD 接模块 TXD。
]]

local mqtt_wendao = require "mqtt_wendao"

-- 串口参数
local UART_ID = 1            -- TTL 使用 UART1
local BAUD = 115200          -- 波特率 115200，8 数据位，1 停止位，无校验
-- 主题后缀（完整主题 = wendao/<IMEI>/<后缀>）
local UP_SUFFIX = "uart/up"    -- 上行：串口收到的数据发到这个主题
local DN_SUFFIX = "uart/down"  -- 下行：平台往这个主题发数据，串口原样输出

-- 串口接收拼包缓冲
local read_buf = ""

-- 50ms 空闲判包：一大包数据可能触发多次接收中断，
-- 连续 50ms 没收到新数据就认为整包结束，再整体上报
local function concat_timeout_func()
    if #read_buf > 0 then
        mqtt_wendao.pub(UP_SUFFIX, read_buf, 1)
        read_buf = ""
    end
end

-- UART 接收回调：收到数据时执行
local function uart_receive_func()
    while true do
        -- 非阻塞读取，最多读 1024 字节
        local s = uart.read(UART_ID, 1024)
        if not s or #s == 0 then
            -- 暂时没数据了，启动 50ms 拼包定时器
            sys.timerStart(concat_timeout_func, 50)
            break
        end
        read_buf = read_buf .. s
    end
end

-- 初始化串口：波特率 115200，数据位 8，停止位 1
uart.setup(UART_ID, BAUD, 8, 1)
-- 注册接收回调
uart.on(UART_ID, "receive", uart_receive_func)

-- 订阅平台下行主题，收到后从串口原样发出
mqtt_wendao.subscribe(DN_SUFFIX)
sys.subscribe("WENDAO_RECV", function(suffix, payload)
    if suffix == DN_SUFFIX then
        uart.write(UART_ID, payload)
    end
end)

log.info("uart_tl", "setup done: uart" .. UART_ID, BAUD .. " 8N1")
log.info("uart_tl", "up topic:   wendao/<IMEI>/" .. UP_SUFFIX)
log.info("uart_tl", "down topic: wendao/<IMEI>/" .. DN_SUFFIX)
