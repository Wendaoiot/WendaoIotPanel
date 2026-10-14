--[[
@module  mqtt_report
@summary MQTT数据上报模块
@version 1.0
@date    2025.07.28

@usage
本模块提供统一的数据上报接口，支持多种上报类型：
1、设备状态上报
2、传感器数据上报
3、事件告警上报
4、OTA进度上报

使用示例：
local mqtt_report = require "mqtt_report"

-- 上报传感器数据
mqtt_report.send_sensor_data({
    temperature = 25.5,
    humidity = 60,
    voltage = 24.1
})

-- 上报设备状态
mqtt_report.send_device_status("online", "正常运行")

-- 上报事件告警
mqtt_report.send_event("temperature_alarm", "温度过高", {threshold=30, current=35})

-- 上报OTA进度
mqtt_report.send_ota_progress(50, "downloading")
]]

local mqtt_report = {}

-- 首次开机时间（使用设备启动时的系统时间）
local first_boot_ts = nil

-- 获取设备启动时间戳
-- 使用rtos.tick()获取设备运行毫秒数，结合当前时间计算开机时间
local function get_boot_timestamp()
    -- 获取当前网络时间（假设已同步）
    local current_ts = os.time()
    
    -- 如果网络时间有效（大于2020年），使用网络时间
    if current_ts > 1577836800 then
        return current_ts
    end
    
    -- 如果网络时间无效，返回nil让后端处理
    return nil
end

-- 获取设备IMEI
local function get_imei()
    return mobile.imei()
end

-- 获取固件版本
local function get_version()
    return VERSION or "unknown"
end

-- 生成消息ID
local function generate_msg_id()
    return "msg_" .. os.time() .. "_" .. math.random(1000, 9999)
end

-- 获取首次开机时间（设备上电时间）
local function get_first_boot_ts()
    if not first_boot_ts then
        first_boot_ts = get_boot_timestamp()
        if first_boot_ts then
            log.info("mqtt_report", "first_boot_ts recorded:", first_boot_ts)
        end
    end
    return first_boot_ts
end

-- 基础上报函数
-- @param is_json boolean 如果为true，payload已经是JSON字符串，不需要再编码
local function send_report(topic_suffix, payload, qos, cb, is_json)
    local topic = "wendao/" .. get_imei() .. "/" .. topic_suffix
    local payload_str = is_json and payload or json.encode(payload)
    sys.publish("SEND_DATA_REQ", "report", topic, payload_str, qos or 0, cb)
end

--[[
@function send_sensor_data
@summary 上报传感器数据
@param data table 传感器数据，键值对形式
@param qos number MQTT QoS等级，可选，默认0
@param cb table 回调函数，可选
@usage
mqtt_report.send_sensor_data({
    temperature = 25.5,
    humidity = 60,
    voltage = 24.1,
    current = 1.2
})
]]
function mqtt_report.send_sensor_data(data, qos, cb)
    -- 手动构建 JSON 字符串，确保字段顺序符合要求
    local id = generate_msg_id()
    local ts = os.time()
    local version = get_version()
    local first_ts = get_first_boot_ts()
    
    -- 如果first_ts为nil（时间未同步），使用当前ts作为备用
    if not first_ts then
        first_ts = ts
        log.warn("mqtt_report", "first_ts not available, using current ts:", ts)
    end
    
    -- 构建 tags JSON 字符串（按固定顺序）
    local tags_order = {"temperature", "humidity", "voltage", "current", "pulse", "GPS_E", "switch"}
    local tags_parts = {}
    for _, key in ipairs(tags_order) do
        if data[key] ~= nil then
            table.insert(tags_parts, string.format('"%s":%s', key, data[key]))
        end
    end
    local tags_json = "{" .. table.concat(tags_parts, ",") .. "}"
    
    -- 构建完整 JSON（按固定顺序）
    local payload_json = string.format(
        '{"id":"%s","ts":%d,"version":"%s","first_ts":%d,"tags":%s}',
        id, ts, version, first_ts, tags_json
    )
    
    -- 直接发送 JSON 字符串，不再重新编码
    send_report("data", payload_json, qos, cb, true)
    log.info("mqtt_report", "send_sensor_data", payload_json)
end

--[[
@function send_device_status
@summary 上报设备状态
@param status string 设备状态：online/offline/error/maintenance
@param message string 状态描述，可选
@param qos number MQTT QoS等级，可选，默认0
@param cb table 回调函数，可选
@usage
mqtt_report.send_device_status("online", "正常运行")
mqtt_report.send_device_status("error", "网络连接失败")
]]
function mqtt_report.send_device_status(status, message, qos, cb)
    local payload = {
        id = generate_msg_id(),
        ts = os.time(),
        status = status,
        message = message or ""
    }
    send_report("status", payload, qos, cb)
    log.info("mqtt_report", "send_device_status", status, message)
end

--[[
@function send_event
@summary 上报事件/告警
@param event_type string 事件类型
@param message string 事件描述
@param details table 事件详情，可选
@param qos number MQTT QoS等级，可选，默认1
@param cb table 回调函数，可选
@usage
mqtt_report.send_event("temperature_alarm", "温度过高", {threshold=30, current=35})
mqtt_report.send_event("power_off", "设备断电")
]]
function mqtt_report.send_event(event_type, message, details, qos, cb)
    local payload = {
        id = generate_msg_id(),
        ts = os.time(),
        type = event_type,
        message = message,
        details = details or {}
    }
    send_report("event", payload, qos or 1, cb)
    log.info("mqtt_report", "send_event", event_type, message)
end

--[[
@function send_ota_progress
@summary 上报OTA升级进度
@param progress number 进度百分比(0-100)
@param status string 当前状态：downloading/upgrading/success/failed
@param message string 状态描述，可选
@param qos number MQTT QoS等级，可选，默认1
@param cb table 回调函数，可选
@usage
mqtt_report.send_ota_progress(50, "downloading")
mqtt_report.send_ota_progress(100, "success", "升级成功")
]]
function mqtt_report.send_ota_progress(progress, status, message, qos, cb)
    local payload = {
        id = generate_msg_id(),
        ts = os.time(),
        progress = progress,
        status = status,
        message = message or ""
    }
    send_report("ota/progress", payload, qos or 1, cb)
    log.info("mqtt_report", "send_ota_progress", progress .. "%", status)
end

--[[
@function send_ota_ack
@summary 上报OTA响应
@param task_id string 任务ID
@param code number 响应码：0成功，其他为错误码
@param message string 响应消息
@param qos number MQTT QoS等级，可选，默认1
@param cb table 回调函数，可选
@usage
mqtt_report.send_ota_ack("task_001", 0, "开始下载")
mqtt_report.send_ota_ack("task_001", -2, "OTA正在进行中")
]]
function mqtt_report.send_ota_ack(task_id, code, message, qos, cb)
    local payload = {
        id = generate_msg_id(),
        ts = os.time(),
        task_id = task_id,
        code = code,
        msg = message
    }
    send_report("ota/ack", payload, qos or 1, cb)
    log.info("mqtt_report", "send_ota_ack", task_id, code, message)
end

--[[
@function send_heartbeat
@summary 上报心跳包
@param qos number MQTT QoS等级，可选，默认0
@param cb table 回调函数，可选
@usage
mqtt_report.send_heartbeat()
]]
function mqtt_report.send_heartbeat(qos, cb)
    local payload = {
        id = generate_msg_id(),
        ts = os.time(),
        type = "heartbeat"
    }
    send_report("heartbeat", payload, qos, cb)
    log.info("mqtt_report", "send_heartbeat")
end

--[[
@function send_raw_data
@summary 上报原始数据（自定义格式）
@param topic_suffix string topic后缀
@param data table 数据内容
@param qos number MQTT QoS等级，可选，默认0
@param cb table 回调函数，可选
@usage
mqtt_report.send_raw_data("custom", {key1="value1", key2="value2"})
]]
function mqtt_report.send_raw_data(topic_suffix, data, qos, cb)
    local payload = {
        id = generate_msg_id(),
        ts = os.time(),
        data = data
    }
    send_report(topic_suffix, payload, qos, cb)
    log.info("mqtt_report", "send_raw_data", topic_suffix)
end

return mqtt_report