--[[

@module  uart3_app
@summary 串口3(UART3)应用功能模块，与UART1功能相同
@version 1.0

@usage
本文件为UART3应用功能模块，核心业务逻辑为：
1、打开uart3，波特率115200，数据位8，停止位1，无奇偶校验位；
2、uart3和pc端的串口工具相连；
3、从uart3接收到pc端串口工具发送的数据后，通知mqtt client进行处理；
4、收到mqtt client从服务器接收到的数据后，将数据通过uart3发送到pc端串口工具；

硬件连接说明：
- UART3_RX: PIN39 (默认引脚)
- UART3_TX: PIN40 (默认引脚)
- 需确认硬件是否支持UART3引脚复用(非LCD模式下可用)

本文件的对外接口有两个：
1、sys.publish("SEND_DATA_REQ", "uart3", mobile.imei().."/uart3/up", read_buf, 1)
   通知mqtt client数据发送模块，在mobile.imei().."/uart3/up"的topic上publish数据
2、sys.subscribe("RECV_DATA_FROM_SERVER", recv_data_from_server_proc)
   订阅RECV_DATA_FROM_SERVER消息，处理消息携带的数据，通过UART3发出
]]


-- 使用UART3
local UART_ID = 3
-- 串口接收数据缓冲区
local read_buf = ""

-- 将前缀prefix和topic，payload数据拼接
-- 然后末尾增加回车换行两个字符，通过uart发送出去，方便在PC端换行显示查看
local function recv_data_from_server_proc(prefix, topic, payload)
    uart.write(UART_ID, prefix..topic..","..payload.."\r\n")
end


local function concat_timeout_func()
    -- 如果存在尚未处理的串口缓冲区数据；
    -- 将数据通过publish通知其他应用功能模块处理；
    -- 然后清空本文件的串口缓冲区数据
    if read_buf:len() > 0 then
        sys.publish("SEND_DATA_REQ", "uart3", mobile.imei().."/uart3/up", read_buf, 1)
        read_buf = ""
    end
end


-- UART3的数据接收中断处理函数，UART3接收到数据时，会执行此函数
local function read()
    local s
    while true do
        -- 非阻塞读取UART3接收到的数据，最长读取1024字节
        s = uart.read(UART_ID, 1024)

        -- 如果从串口没有读到数据
        if not s or s:len() == 0 then
            -- 启动50毫秒的定时器，如果50毫秒内没收到新的数据，则处理当前收到的所有数据
            -- 这样处理是为了防止将一大包数据拆分成多个小包来处理
            sys.timerStart(concat_timeout_func, 50)
            -- 跳出循环，退出本函数
            break
        end

        log.info("uart3_app.read len", s:len())
        -- log.info("uart3_app.read", s)

        -- 将本次从串口读到的数据拼接到串口缓冲区read_buf中
        read_buf = read_buf..s
    end
end


-- 初始化UART3，波特率115200，数据位8，停止位1
-- 注意: UART3默认引脚为PIN39(RX)/PIN40(TX)，需在非LCD模式下可用
-- 如需自定义引脚，请使用pins.setup()先配置
uart.setup(UART_ID, 115200, 8, 1)

-- 注册UART3的数据接收中断处理函数，UART3接收到数据时，会执行read函数
uart.on(UART_ID, "receive", read)

-- 订阅"RECV_DATA_FROM_SERVER"消息的处理函数recv_data_from_server_proc
-- 收到"RECV_DATA_FROM_SERVER"消息后，会执行函数recv_data_from_server_proc
-- 注意: UART1和UART3都订阅了此消息，都会收到MQTT下发的数据
sys.subscribe("RECV_DATA_FROM_SERVER", recv_data_from_server_proc)

log.info("uart3_app", "init done, uart3 at 115200bps")
