--[[
@module  mqtt_ota
@summary MQTT OTA升级模块
@version 1.0
@date    2026.06.16
@usage
本模块实现基于MQTT的OTA升级功能：
1. 接收服务端下发的升级指令；
2. 下载并验证固件；
3. 执行升级并上报状态。

协议格式：
- 升级指令: Topic: wendao/{deviceID}/ota
- 设备响应: Topic: wendao/{deviceID}/ota/ack
- 状态上报: Topic: wendao/{deviceID}/ota/status
]]

local libfota2 = require "libfota2"

local mqtt_ota = {}

local ota_running = false
local pending_request = nil
local ota_timeout_ms = 0

local function send_ota_ack(request_id, code, msg, progress)
    local ack_data = {
        id = request_id,
        code = code,
        msg = msg,
        progress = progress or 0
    }
    local ack_topic = "wendao/" .. mobile.imei() .. "/ota/ack"
    local ack_payload = json.encode(ack_data)
    
    sys.publish("SEND_DATA_REQ", "ota_ack", ack_topic, ack_payload, 0)
    log.info("mqtt_ota", "send_ota_ack", ack_topic, ack_payload)
end

local function send_ota_status(status)
    local status_topic = "wendao/" .. mobile.imei() .. "/ota/status"
    sys.publish("SEND_DATA_REQ", "ota_status", status_topic, status, 0)
    log.info("mqtt_ota", "send_ota_status", status_topic, status)
end

local function is_valid_url(url)
    if type(url) ~= "string" or url == "" then
        return false, "url empty"
    end
    if not url:match("^https?://") then
        return false, "url format error"
    end
    return true
end

local function ota_cb(ret)
    log.info("mqtt_ota", "fota result", ret)
    ota_running = false
    ota_timeout_ms = 0
    
    local request_id = pending_request and pending_request.id or ""
    
    if ret == 0 then
        send_ota_status("upgrading")
        send_ota_ack(request_id, 0, "upgrading", 100)
        log.info("mqtt_ota", "升级包下载成功,即将重启模块")
        sys.timerStart(rtos.reboot, 1000)
    elseif ret == 1 then
        send_ota_status("failed")
        send_ota_ack(request_id, 1, "connect failed")
        log.info("mqtt_ota", "连接失败,请检查url或服务器配置")
    elseif ret == 2 then
        send_ota_status("failed")
        send_ota_ack(request_id, 2, "url error")
        log.info("mqtt_ota", "url错误,检查url拼写")
    elseif ret == 3 then
        send_ota_status("failed")
        send_ota_ack(request_id, 3, "server disconnected")
        log.info("mqtt_ota", "服务器断开,检查服务器白名单配置")
    elseif ret == 4 then
        send_ota_status("failed")
        send_ota_ack(request_id, 4, "upgrade package invalid")
        log.error("mqtt_ota", "升级报文错误或升级包无效")
    elseif ret == 5 then
        send_ota_status("failed")
        send_ota_ack(request_id, 5, "version format error")
        log.info("mqtt_ota", "版本号格式错误")
    else
        send_ota_status("failed")
        send_ota_ack(request_id, ret, "unknown error")
        log.info("mqtt_ota", "未定义返回值", ret)
    end
    
    pending_request = nil
end

local function ota_timeout_check()
    if ota_running and pending_request then
        log.warn("mqtt_ota", "OTA超时")
        send_ota_status("failed")
        send_ota_ack(pending_request.id, pending_request.task_id, -1, "timeout")
        ota_running = false
        ota_timeout_ms = 0
        pending_request = nil
    end
end

local function start_ota(request)
    if not request or not request.id or not request.url then
        log.error("mqtt_ota", "invalid request")
        return
    end
    
    local ok_url, url_reason = is_valid_url(request.url)
    if not ok_url then
        log.warn("mqtt_ota", "invalid firmware url", url_reason)
        send_ota_ack(request.id, -1, "invalid url: " .. url_reason)
        return
    end
    
    -- 发送开始下载响应
    send_ota_status("downloading")
    send_ota_ack(request.id, 0, "downloading", 0)
    
    -- 保存请求信息（仅保存必要字段）
    pending_request = {
        id = request.id,
        url = request.url,
        version = request.version,
    }
    
    ota_running = true
    ota_timeout_ms = request.timeout or 300000
    
    -- 只使用url进行升级，忽略md5和size
    local opts = {
        url = "###" .. request.url,
        version = request.version or VERSION,
        timeout = ota_timeout_ms,
    }
    
    log.info("mqtt_ota", "start ota", request.url, request.version)
    sys.timerStart(ota_timeout_check, ota_timeout_ms)
    libfota2.request(ota_cb, opts)
end

function mqtt_ota.handle_ota_command(topic, payload)
    if ota_running then
        log.warn("mqtt_ota", "OTA is already running")
        local ok, data = pcall(json.decode, payload)
        if ok and data and data.id then
            send_ota_ack(data.id, -2, "OTA in progress")
        end
        return
    end
    
    local ok, data = pcall(json.decode, payload)
    if not ok or not data then
        log.error("mqtt_ota", "invalid payload")
        return
    end
    
    if not data.id or not data.url then
        log.error("mqtt_ota", "missing required fields")
        if data.id then
            send_ota_ack(data.id, -1, "missing required fields")
        end
        return
    end
    
    log.info("mqtt_ota", "received ota command", json.encode(data))
    start_ota(data)
end

function mqtt_ota.is_running()
    return ota_running
end

return mqtt_ota