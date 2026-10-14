--[[
@module  mqtt_wendao
@summary wendaoiot 平台 MQTT 连接（极简单文件版）
@version 1.0

@usage
本模块负责连接 wendaoiot MQTT 平台、断线自动重连，并提供 3 个对外接口：

1. mqtt_wendao.subscribe(suffix, qos)
   订阅平台下行主题。完整主题 = <前缀>/<IMEI>/<suffix>，默认前缀 wendao。
   例：mqtt_wendao.subscribe("uart/down")
   平台下发到 wendao/<IMEI>/uart/down 的消息会广播 WENDAO_RECV：
       sys.subscribe("WENDAO_RECV", function(suffix, payload)
           -- suffix 为主题后缀(如 "uart/down")，payload 为消息内容(字符串)
       end)

2. mqtt_wendao.pub(suffix, data, qos)
   向 wendao/<IMEI>/<suffix> 发布原始字符串。
   例：mqtt_wendao.pub("uart/up", "hello", 1)

3. mqtt_wendao.report(tags)
   向 wendao/<IMEI>/data 发布 JSON 传感器数据。
   例：mqtt_wendao.report({ temperature = 25.6, voltage = 24.1 })
   实际发送：{"id":"msg_...","ts":1756700000,"tags":{"temperature":25.6,"voltage":24.1}}

设备身份 = 模组 IMEI（mobile.imei()），平台依据 IMEI 识别设备，无需用户名密码。
更换服务器/平台时，只需要修改下面【配置区】的 3 个常量。
]]

local mqtt_wendao = {}

-- ===================== 配置区（换平台只改这里）=====================
local HOST = "panel.wendaoiot.com"    -- MQTT 服务器地址（IP 或域名）
local PORT = 1883              -- 端口（明文 TCP 为 1883）
local TOPIC_PREFIX = "wendao"  -- 主题前缀，完整主题 = <前缀>/<IMEI>/<功能>
-- ==================================================================

local IMEI = mobile.imei()
local BASE = TOPIC_PREFIX .. "/" .. IMEI   -- 例：wendao/862288082694066
local TASK_NAME = "mqtt_wendao"

-- 待发送队列：断网期间暂存，最多 32 条，满了丢弃最旧的
local pub_queue = {}
local QUEUE_MAX = 32
-- 下行订阅注册表：suffix -> qos
local down_subs = {}
-- 运行状态
local mqtt_client = nil
local sending = false   -- qos>0 的消息已发出、正在等 "sent" 事件

-- 数字格式化：整数不带小数点，浮点保留 3 位小数并去掉尾零
local function fmt_num(v)
    if v == math.floor(v) then
        return tostring(v)
    end
    return (string.format("%.3f", v):gsub("0+$", ""):gsub("%.$", ""))
end

------------------------------------------------------------------
-- 对外接口
------------------------------------------------------------------

-- 订阅下行主题后缀，如 mqtt_wendao.subscribe("uart/down")；qos 可选，默认 0
function mqtt_wendao.subscribe(suffix, qos)
    down_subs[suffix] = qos or 0
end

-- 发布原始字符串到 <BASE>/<suffix>
function mqtt_wendao.pub(suffix, data, qos)
    table.insert(pub_queue, { topic = BASE .. "/" .. suffix, data = tostring(data), qos = qos or 0 })
    if #pub_queue > QUEUE_MAX then
        table.remove(pub_queue, 1)
        log.warn("mqtt_wendao", "queue full, drop oldest")
    end
    sys.sendMsg(TASK_NAME, "EV", "pub")
end

-- 发布 JSON 传感器数据到 <BASE>/data
-- tags 为键值表，如 { current_ma = 12.3, percent = 50 }
function mqtt_wendao.report(tags)
    local parts = {}
    for k, v in pairs(tags) do
        if type(v) == "number" then
            table.insert(parts, '"' .. k .. '":' .. fmt_num(v))
        else
            table.insert(parts, '"' .. k .. '":"' .. tostring(v) .. '"')
        end
    end
    local payload = string.format('{"id":"msg_%d_%d","ts":%d,"tags":{%s}}',
        os.time(), math.random(1000, 9999), os.time(), table.concat(parts, ","))
    mqtt_wendao.pub("data", payload, 1)
end

------------------------------------------------------------------
-- 内部实现
------------------------------------------------------------------

-- 从完整主题中取出后缀：BASE.."/"..suffix → suffix
local function topic_suffix(topic)
    local prefix = BASE .. "/"
    if topic:sub(1, #prefix) == prefix then
        return topic:sub(#prefix + 1)
    end
    return nil
end

-- 尝试发送队列中的数据；qos>0 时发出一条后等待 "sent" 事件再发下一条
local function flush_queue()
    while mqtt_client and #pub_queue > 0 and not sending do
        local item = pub_queue[1]
        if mqtt_client:publish(item.topic, item.data, item.qos) then
            if item.qos > 0 then
                sending = true   -- 等 "sent" 事件确认后再发下一条
            else
                table.remove(pub_queue, 1)   -- qos0 发出即完成
            end
            break
        else
            log.warn("mqtt_wendao", "publish fail, retry later")
            break
        end
    end
end

-- mqtt 事件回调（运行在 mqtt 内部线程，只转发消息，不要在这里阻塞等待）
local function mqtt_event_cbfunc(client, event, data, payload)
    if event == "conack" then
        -- MQTT 连接鉴权成功
        sys.sendMsg(TASK_NAME, "EV", "conack")
    elseif event == "recv" then
        -- 服务器下发：data=完整主题，payload=消息内容
        log.info("mqtt_wendao", "recv", data, payload)
        local suffix = topic_suffix(data)
        if suffix then
            sys.publish("WENDAO_RECV", suffix, payload)
        end
    elseif event == "sent" then
        -- 一条 qos>0 消息发送成功
        sys.sendMsg(TASK_NAME, "EV", "sent")
    elseif event == "disconnect" then
        -- 被服务器/网络断开
        sys.sendMsg(TASK_NAME, "EV", "close")
    elseif event == "error" then
        -- 严重异常（连接失败/发送失败等）
        log.error("mqtt_wendao", "error", data)
        sys.sendMsg(TASK_NAME, "EV", "close")
    end
end

-- 主任务：等待联网 → 连接 → 订阅/收发循环 → 异常后 5 秒自动重连
sys.taskInitEx(function()
    while true do
        -- 等待 4G 注网成功（内核自动注网，IP_READY 后 socket 可用）
        -- 此处 1000ms 超时不要改长（与官方 demo 一致，兼容多网卡场景）
        while not socket.adapter(socket.dft()) do
            sys.waitUntil("IP_READY", 1000)
        end

        sys.cleanMsg(TASK_NAME)
        sending = false

        mqtt_client = mqtt.create(nil, HOST, PORT)
        if not mqtt_client then
            log.error("mqtt_wendao", "mqtt.create error")
            sys.wait(5000)
        else
            -- ClientID 带 IMEI，平台据此识别设备；用户名/密码为空；clean session
            mqtt_client:auth("mqtt_main" .. IMEI, "", "", true)
            mqtt_client:on(mqtt_event_cbfunc)
            mqtt_client:keepalive(120)
            -- 遗嘱消息：设备异常断线时平台自动收到 offline
            mqtt_client:will(BASE .. "/status", '{"status":"offline"}')

            log.info("========================================")
            log.info("mqtt_wendao connecting:", HOST .. ":" .. PORT)
            log.info("DEVICE IMEI:", IMEI)
            log.info("========================================")

            if mqtt_client:connect() then
                -- 连接成功后的事件循环
                while true do
                    local msg = sys.waitMsg(TASK_NAME, "EV")
                    local ev = msg[2]

                    if ev == "conack" then
                        log.info("mqtt_wendao", "connected")
                        -- 订阅所有已注册的下行主题
                        local topics = {}
                        for suffix, qos in pairs(down_subs) do
                            topics[BASE .. "/" .. suffix] = qos
                        end
                        if next(topics) then
                            mqtt_client:subscribe(topics)
                        end
                        flush_queue()

                    elseif ev == "sent" then
                        sending = false
                        table.remove(pub_queue, 1)
                        flush_queue()

                    elseif ev == "pub" then
                        flush_queue()

                    elseif ev == "close" then
                        break
                    end
                end
            end

            -- 异常处理：关闭连接，5 秒后重连
            sys.cleanMsg(TASK_NAME)
            mqtt_client:close()
            mqtt_client = nil
            sending = false
            sys.wait(5000)
        end
    end
end, TASK_NAME)

return mqtt_wendao
