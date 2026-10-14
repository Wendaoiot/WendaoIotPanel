--[[
@module  rs485_test
@summary RS485串口测试功能模块
@version 1.0

@usage
本文件为RS485串口测试功能模块，核心业务逻辑为：
1、初始化UART2，波特率9600，数据位8，停止位1，无奇偶校验位；
2、使用GPIO33作为RS485方向控制引脚(DIR)，高电平=发送，低电平=接收；
3、启动一个task，每2秒通过RS485发送一次测试数据；
4、注册UART2数据接收中断处理函数，收到数据后打印日志；

硬件连接说明：
- RS485_DIR: GPIO33 (方向控制，高=发送，低=接收)
- UART2_TX:  连接RS485收发器的DI引脚
- UART2_RX:  连接RS485收发器的RO引脚
- 485-A、485-B: 连接RS485总线
- EN1(GPIO37)需拉高使能RS485_5V电源(在main.lua中已配置)

参考官方demo: D:\panel\780epm_common\uart\485_uart.lua
]]


-- 使用UART2作为RS485串口
local UART_ID = 2
-- RS485方向控制引脚(DIR): 高=发送, 低=接收
local RS485_DIR_PIN = 33
-- uart.setup第9个参数: RS485接收方向时DIR引脚的电平
-- 注意: 该参数是"接收时"的电平, 不是发送时的电平!
-- 硬件高=发送 → 接收时DIR应为低 → 此处填0
-- 官方文档: https://docs.openluat.com/air780e/luatos/app/driver/uart/rs485/
local RS485_RX_DIR_LEVEL = 0
-- 发送完成后保持DIR为发送方向的时间(微秒)
-- 官方demo使用2000us(2ms)，过大会影响接收方向的及时切换
local RS485_DIR_DELAY = 2000
-- 测试数据发送周期(毫秒)
local SEND_INTERVAL_MS = 2000
-- 发送测试数据计数
local send_count = 0


-- UART2的数据接收中断处理函数，UART2接收到数据时，会执行此函数
-- 回调签名: function(id, len)，id为串口号，len为接收到的数据长度
local function read(id, len)
    log.info("rs485_test.receive", "uart"..id, "len", len)
    local s = ""
    repeat
        -- 非阻塞读取UART2接收到的数据，最长读取1024字节
        s = uart.read(id, 1024)
        if s and #s > 0 then
            log.info("rs485_test.read len", #s)
            log.info("rs485_test.read data", s)
            -- 如传输的是十六进制/二进制数据，可使用下面的方式打印
            -- log.info("rs485_test.read hex", s:toHex())
        end
    until s == ""
end


-- UART2发送完成回调函数
local function sent_cb(id)
    log.info("rs485_test.sent", "uart"..id, "send done")
end


-- 挂起标志，休眠期间停止周期发送(配合sleep_test模块的低功耗休眠)
local suspended = false

-- UART2初始化函数
-- sleep_test休眠时会uart.close(2)以消除RX悬空噪声，恢复时需重新初始化
local function setup_uart()
    -- 初始化UART2，波特率9600，数据位8，停止位1，无奇偶校验位
    -- 启用485模式: 方向控制引脚GPIO33，接收时DIR=低(即发送时DIR=高)，发送完成后延时2ms切换回接收方向
    uart.setup(UART_ID, 9600, 8, 1, uart.NONE, uart.LSB, 1024, RS485_DIR_PIN, RS485_RX_DIR_LEVEL, RS485_DIR_DELAY)

    -- 注册UART2的数据接收中断处理函数，UART2接收到数据时，会执行read函数
    uart.on(UART_ID, "receive", read)

    -- 注册UART2的发送完成回调函数，UART2数据发送完成时，会执行sent_cb函数
    uart.on(UART_ID, "sent", sent_cb)

    log.info("rs485_test.uart", "uart"..UART_ID.." setup done")
end

sys.subscribe("APP_SUSPEND", function()
    suspended = true
end)

sys.subscribe("APP_RESUME", function()
    suspended = false
    -- sleep_test休眠时已uart.close(2)，此处重新初始化
    setup_uart()
end)


-- RS485测试task处理函数
-- 周期性通过RS485发送测试数据
local function rs485_test_task_func()
    -- 等待2秒，让RS485收发器上电稳定
    sys.wait(2000)

    while true do
        if suspended then
            -- 已挂起: 阻塞等待恢复消息，task完全阻塞有利于系统进入休眠
            sys.waitUntil("APP_RESUME")
        else
            send_count = send_count + 1
            -- 拼接测试数据
            local test_data = string.format("rs485 test #%d, imei=%s\r\n", send_count, mobile.imei())
            log.info("rs485_test.send", "count", send_count, "len", #test_data)

            -- 通过UART2发送数据
            -- uart.write会自动控制GPIO33的方向(发送时拉高，发送完成后拉低)
            uart.write(UART_ID, test_data)

            -- 等待下一次发送
            sys.wait(SEND_INTERVAL_MS)
        end
    end
end


-- 初始化UART2(开机首次调用，休眠唤醒后由APP_RESUME消息触发重新初始化)
setup_uart()

log.info("rs485_test", "init done, uart"..UART_ID..", dir=GPIO"..RS485_DIR_PIN..", rx_level="..RS485_RX_DIR_LEVEL.." (low=rx, high=tx)")

-- 创建并且启动一个task，运行RS485测试任务
sys.taskInit(rs485_test_task_func)
