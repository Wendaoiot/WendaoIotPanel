--[[
@module  mqtt_receiver
@summary mqtt client数据接收处理应用功能模块
@version 1.0

@usage
本文件为mqtt client 数据接收应用功能模块，核心业务逻辑为：
处理接收到的publish数据，同时将数据发送给其他应用功能模块做进一步处理；

本文件的对外接口有2个：
1、mqtt_receiver.proc(topic, payload, metas)：publish数据处理入口，在mqtt_main.lua中调用；
2、sys.publish("RECV_DATA_FROM_SERVER", "recv from mqtt server: ", topic, payload)：
   将接收到的publish中的topic和payload数据通过消息"RECV_DATA_FROM_SERVER"发布出去；
   需要处理数据的应用功能模块订阅处理此消息即可，本demo项目中uart_app.lua中订阅处理了本消息；
]]

local mqtt_receiver = {}

-- 加载mqtt_sender模块，用于发送应答消息
local mqtt_sender = require "mqtt_sender"

-- 加载OTA模块
local mqtt_ota = require "mqtt_ota"


--[[
处理接收到的publish数据

@api mqtt_receiver.proc(topic, payload, metas)

@param1 topic string
表示publish主题

@param2 payload string
表示publish数据负载

@param2 payload string
表示publish数据负载

@param3 metas table
表示publish报文的一些参数；格式如下：
{
    qos: number类型，取值范围0,1,2
    retain：number类型，取值范围0,1
    dup：number类型，取值范围0,1
    message_id: number类型
}

@return1 result nil

@usage

mqtt_receiver.proc(topic, payload, metas)
]]
function mqtt_receiver.proc(topic, payload, metas)

    log.info("mqtt_receiver.proc", topic, payload:len(), json.encode(metas))

    -- 接收到数据，通知网络环境检测看门狗功能模块进行喂狗
    sys.publish("FEED_NETWORK_WATCHDOG")

    -- 检查是否是OTA升级指令topic
    local ota_topic_pattern = "^wendao/"..mobile.imei().."/ota$"
    if topic:match(ota_topic_pattern) then
        log.info("mqtt_receiver.proc", "OTA command received")
        mqtt_ota.handle_ota_command(topic, payload)
        return
    end

    -- 检查是否是控制命令topic
    local control_topic_pattern = "^wendao/"..mobile.imei().."/control$"
    if topic:match(control_topic_pattern) then
        -- 解析控制命令
        local success, data = pcall(json.decode, payload)
        if success and data and data.id then
            log.info("mqtt_receiver.proc", "control command received", json.encode(data))

            -- 执行控制操作（这里可以根据实际需求控制GPIO等）
            if data.tags then
                for key, value in pairs(data.tags) do
                    log.info("mqtt_receiver.proc", "control", key, value)
                    -- TODO: 在此处添加实际的控制逻辑，例如控制GPIO
                    -- if key == "relay1" then
                    --     gpio.setup(10, value and 1 or 0, gpio.PULLUP)
                    -- end
                end
            end

            -- 发送应答消息
            local ack_data = {
                id = data.id,
                code = 0,
                msg = "ok"
            }
            local ack_topic = "wendao/"..mobile.imei().."/control/ack"
            local ack_payload = json.encode(ack_data)

            -- 通过mqtt_sender发送应答
            sys.publish("SEND_DATA_REQ", "control_ack", ack_topic, ack_payload, 0)
            log.info("mqtt_receiver.proc", "control ack sent", ack_topic, ack_payload)
        else
            log.error("mqtt_receiver.proc", "invalid control payload", payload)
        end
    else
        -- 将topic和payload通过"RECV_DATA_FROM_SERVER"消息publish出去，给其他应用模块处理
        sys.publish("RECV_DATA_FROM_SERVER", "recv from mqtt server: ", topic, payload)
    end
end

return mqtt_receiver
