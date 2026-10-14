--[[
@module  timer_app
@summary 定时器应用功能模块
@version 1.0

@usage
本文件为定时器应用功能模块，核心业务逻辑为：
创建一个1秒的循环定时器，每次产生一段传感器数据，通过mqtt_report模块上报；

使用mqtt_report模块提供统一的上报接口；
]]

-- 加载mqtt上报模块
local mqtt_report = require "mqtt_report"

-- 重试计数器
local retry_count = 0
local max_retry = 3

-- 数据发送结果回调函数
-- result：发送结果，true为发送成功，false为发送失败
-- para：回调参数
local function send_data_cbfunc(result, para)
    log.info("send_data_cbfunc", result, para, "retry:", retry_count)
    
    if result then
        -- 发送成功，重置重试计数，继续下一条
        retry_count = 0
        sys.timerStart(send_data_req_timer_cbfunc, 1000)
    else
        -- 发送失败，增加重试计数
        retry_count = retry_count + 1
        
        if retry_count < max_retry then
            -- 立即重试，不等待
            log.info("retry immediately", retry_count)
            send_data_req_timer_cbfunc()
        else
            -- 达到最大重试次数，延迟后继续
            log.error("max retry reached, delay continue")
            retry_count = 0
            sys.timerStart(send_data_req_timer_cbfunc, 1000)
        end
    end
end

-- 定时器回调函数
function send_data_req_timer_cbfunc()
    -- 构建传感器数据（按模板顺序）
    local sensor_data = {
        temperature = 29.0 + math.random() * 2.0,
        humidity = 60 + math.random() * 8.0,
        voltage = 24.0 + math.random() * 0.8,
        current = 1.0 + math.random() * 0.2,
        pulse = math.random(150, 300),
        GPS_E = 113.0 + math.random() * 0.05,
        switch = math.random(0, 20)
    }
    
    -- 使用统一上报接口发送传感器数据，QoS=1确保消息可靠传输
    mqtt_report.send_sensor_data(sensor_data, 1, {func=send_data_cbfunc, para="timer"})
end

-- 启动一个1秒的单次定时器
-- 时间到达后，执行一次send_data_req_timer_cbfunc函数
sys.timerStart(send_data_req_timer_cbfunc, 1000)