--[[
@module  hw_watchdog
@summary 硬件看门狗喂狗功能模块
@version 1.0

@usage
本文件为硬件看门狗喂狗功能模块，核心业务逻辑为：
1、初始化GPIO27为输出模式，作为硬件看门狗电路的喂狗信号引脚；
2、启动一个task，周期性反转GPIO27电平，向硬件看门狗电路发送喂狗信号；
3、硬件看门狗电路在超时时间内未收到电平翻转信号，则会触发系统复位；

喂狗周期说明：
- 硬件看门狗芯片的典型超时时间一般为1秒~10秒不等(具体取决于硬件电路设计)；
- 喂狗周期必须小于硬件看门狗的超时时间，推荐取超时时间的一半；
- 本demo默认1秒翻转一次，如果实际硬件看门狗超时时间较短，请相应减小FEED_INTERVAL_MS的值；

本文件没有对外接口，直接在main.lua中require "hw_watchdog"就可以加载运行；
]]


-- 硬件看门狗喂狗信号引脚
local HW_WATCHDOG_PIN = 27

-- 喂狗周期(毫秒)，电平反转间隔
-- 默认1000毫秒(1秒)反转一次，可根据实际硬件看门狗芯片的超时时间调整
local FEED_INTERVAL_MS = 1000

-- 当前电平状态，0表示低电平，1表示高电平
local feed_level = 0


-- 硬件看门狗喂狗task处理函数
-- 周期性反转GPIO27电平，向硬件看门狗电路发送喂狗信号
local function hw_watchdog_task_func()
    -- 初始化GPIO27为输出模式，默认输出低电平
    gpio.setup(HW_WATCHDOG_PIN, 0)
    log.info("hw_watchdog", "gpio"..HW_WATCHDOG_PIN.." init done, feed interval", FEED_INTERVAL_MS, "ms")

    while true do
        -- 反转电平
        feed_level = (feed_level == 0) and 1 or 0
        gpio.set(HW_WATCHDOG_PIN, feed_level)
        log.info("hw_watchdog", "feed, level", feed_level)
        -- 等待下一次喂狗
        sys.wait(FEED_INTERVAL_MS)
    end
end


-- 创建并且启动一个task
-- 运行这个task的处理函数hw_watchdog_task_func
sys.taskInit(hw_watchdog_task_func)
