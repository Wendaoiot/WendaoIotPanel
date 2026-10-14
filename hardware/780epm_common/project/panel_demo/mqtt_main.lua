--[[
@module  mqtt_main
@summary mqtt client 主应用功能模块
@version 1.0
@date    2025.07.28
@author  马梦阳
@usage
本文件为mqtt client 主应用功能模块，核心业务逻辑为：
1、创建一个mqtt client，连接server；
2、处理连接/订阅/取消订阅/异常逻辑，出现异常后执行重连动作；
3、调用mqtt_receiver的外部接口mqtt_receiver.proc，对接收到的publish数据进行处理；
4、调用sys.sendMsg接口，发送"CONNECT OK"、"PUBLISH OK"和"DISCONNECTED"三种类型的"MQTT_EVENT"消息到mqtt_sender的task，控制publish数据发送逻辑；
5、收到MQTT心跳应答后，执行sys.publish("FEED_NETWORK_WATCHDOG") 对网络环境检测看门狗功能模块进行喂狗；

本文件没有对外接口，直接在main.lua中require "mqtt_main"就可以加载运行；
]]


-- 加载mqtt client数据接收功能模块
local mqtt_receiver = require "mqtt_receiver"
-- 加载mqtt client数据发送功能模块
local mqtt_sender = require "mqtt_sender"

-- mqtt服务器地址和端口
local SERVER_ADDR = "panel.wendaoiot.com"
local SERVER_PORT = 1883

-- mqtt_main的任务名
local TASK_NAME = mqtt_sender.TASK_NAME_PREFIX.."main"

-- PSM+短连接测试支持(psm_test.lua)：
-- 收到"MQTT_STOP_REQ"后，mqtt main task在下次回到循环开头时阻塞等待，
-- 不再自动重连，防止MQTT重连注网活动阻止系统进入PSM+模式；
-- 收到"MQTT_START_REQ"后恢复自动重连；
-- PSM+唤醒后软件系统直接重启，本变量自动恢复默认值false，无需手动恢复
local mqtt_stop_wait = false
sys.subscribe("MQTT_STOP_REQ", function()
    mqtt_stop_wait = true
    log.info("mqtt_client_main_task_func", "recv MQTT_STOP_REQ, stop auto reconnect")
end)
sys.subscribe("MQTT_START_REQ", function()
    mqtt_stop_wait = false
    sys.publish("MQTT_RESUME_NOTIFY")
    log.info("mqtt_client_main_task_func", "recv MQTT_START_REQ, resume auto reconnect")
end)

-- mqtt主题的前缀：IMEI号
local TOPIC_PREFIX = mobile.imei()

-- mqtt client的事件回调函数
local function mqtt_client_event_cbfunc(mqtt_client, event, data, payload, metas)
    log.info("mqtt_client_event_cbfunc", mqtt_client, event, data, payload, json.encode(metas))

    -- mqtt连接成功
    if event == "conack" then
        sys.sendMsg(TASK_NAME, "MQTT_EVENT", "CONNECT", true)
        -- 订阅多主题
        -- 表中的每一个订阅主题的格式为[topic]=qos
        if not mqtt_client:subscribe(
                {
                    ["wendao/"..TOPIC_PREFIX.."/control"]=0,
                    ["wendao/"..TOPIC_PREFIX.."/data/ack"]=0,
                    ["wendao/"..TOPIC_PREFIX.."/ota"]=0
                }
        ) then
            sys.sendMsg(TASK_NAME, "MQTT_EVENT", "SUBSCRIBE", false, -1)
        end

    -- 订阅结果
    -- data：订阅应答结果，true为成功，false为失败
    -- payload：number类型；成功时表示qos，取值范围为0,1,2；失败时表示失败码，一般是0x80
    elseif event == "suback" then
        -- 发送消息通知 mqtt main task
        sys.sendMsg(TASK_NAME, "MQTT_EVENT", "SUBSCRIBE", data, payload)

    -- 取消订阅成功
    elseif event == "unsuback" then
        -- 发送消息通知 mqtt main task
        sys.sendMsg(TASK_NAME, "MQTT_EVENT", "UNSUBSCRIBE", true)

    -- 接收到服务器下发的publish数据
    -- data：string类型，表示topic
    -- payload：string类型，表示payload
    -- metas：table类型，数据内容如下
    -- {
    --     qos: number类型，取值范围0,1,2
    --     retain：number类型，取值范围0,1
    --     dup：number类型，取值范围0,1
    --     message_id: number类型
    -- }
    elseif event == "recv" then
        -- 对接收到的publish数据处理
        mqtt_receiver.proc(data, payload, metas)

    -- 发送成功publish数据
    -- data：number类型，表示message id
    elseif event == "sent" then
        -- 发送消息通知 mqtt sender task
        sys.sendMsg(mqtt_sender.TASK_NAME, "MQTT_EVENT", "PUBLISH_OK", data)

    -- 服务器断开mqtt连接
    elseif event == "disconnect" then
        -- 发送消息通知 mqtt main task
        sys.sendMsg(TASK_NAME, "MQTT_EVENT", "DISCONNECTED", false)

    -- 收到服务器的心跳应答
    elseif event == "pong" then
        -- 接收到数据，通知网络环境检测看门狗功能模块进行喂狗
        sys.publish("FEED_NETWORK_WATCHDOG")

    -- 严重异常，本地会主动断开连接
    -- data：string类型，表示具体的异常，有以下几种：
    --       "connect"：tcp连接失败
    --       "tx"：数据发送失败
    --       "conack"：mqtt connect后，服务器应答CONNACK鉴权失败，失败码为payload（number类型）
    --       "other"：其他异常
    elseif event == "error" then
        if data == "connect" or data == "conack" then
            -- 发送消息通知 mqtt main task，连接失败
            sys.sendMsg(TASK_NAME, "MQTT_EVENT", "CONNECT", false)
        elseif data == "other" or data == "tx" then
            -- 发送消息通知 mqtt main task，出现异常
            sys.sendMsg(TASK_NAME, "MQTT_EVENT", "ERROR")
        end
    end
end

-- mqtt main task 的任务处理函数
local function mqtt_client_main_task_func()

    local mqtt_client
    local result, msg

    while true do
        -- PSM+短连接测试: 已请求停止时阻塞等待恢复请求，不再自动重连
        -- task阻塞期间允许系统进入休眠/PSM+模式
        if mqtt_stop_wait then
            log.info("mqtt_client_main_task_func", "stopped, wait MQTT_START_REQ")
            sys.waitUntil("MQTT_RESUME_NOTIFY")
        end

        -- 如果当前时间点设置的默认网卡还没有连接成功，一直在这里循环等待
        while not socket.adapter(socket.dft()) do
            -- log.warn("mqtt_client_main_task_func", "wait IP_READY", socket.dft())  -- 已关闭频繁日志
            -- 在此处阻塞等待默认网卡连接成功的消息"IP_READY"
            -- 或者等待1秒超时退出阻塞等待状态;
            -- 注意：此处的1000毫秒超时不要修改的更长；
            -- 因为当使用exnetif.set_priority_order配置多个网卡连接外网的优先级时，会隐式的修改默认使用的网卡
            -- 当exnetif.set_priority_order的调用时序和此处的socket.adapter(socket.dft())判断时序有可能不匹配
            -- 此处的1秒，能够保证，即使时序不匹配，也能1秒钟退出阻塞状态，再去判断socket.adapter(socket.dft())
            sys.waitUntil("IP_READY", 1000)
        end

        -- 检测到了IP_READY消息
        log.info("mqtt_client_main_task_func", "recv IP_READY", socket.dft())

        -- 清空此task绑定的消息队列中的未处理的消息
        sys.cleanMsg(TASK_NAME)

        -- 创建mqtt client对象
        mqtt_client = mqtt.create(nil, SERVER_ADDR, SERVER_PORT)
        -- 如果创建mqtt client对象失败
        if not mqtt_client then
            log.error("mqtt_client_main_task_func", "mqtt.create error")
            goto EXCEPTION_PROC
        end

        -- 配置mqtt client对象的client id，username，password和clean session标志
        result = mqtt_client:auth(TASK_NAME..mobile.imei(), "", "", true)
        -- 如果配置失败
        if not result then
            log.error("mqtt_client_main_task_func", "mqtt_client:auth error")
            goto EXCEPTION_PROC
        end

        -- 打印设备ID，便于识别
        log.info("========================================")
        log.info("DEVICE CONNECTING:", mobile.imei())
        log.info("MQTT SERVER:", SERVER_ADDR .. ":" .. SERVER_PORT)
        log.info("========================================")

        -- 注册mqtt client对象的事件回调函数
        mqtt_client:on(mqtt_client_event_cbfunc)

        -- 设置mqtt keepalive时间为600秒(配合sleep_test低功耗休眠)
        -- 休眠期间固件每180秒(默认值)自动发送MQTT心跳，会导致modem从休眠唤醒进入
        -- 4G发射态，产生23-28mA的周期性电流峰值；
        -- 本工程40秒定时唤醒上报本身就会刷新MQTT活跃度，600秒兜底足够安全
        -- (MQTT协议中发送任何报文含PUBLISH都会重置keepalive计时)
        mqtt_client:keepalive(600)

        -- 设置遗嘱消息，设备断开连接时自动发送离线状态
        mqtt_client:will("wendao/" .. TOPIC_PREFIX .. "/status", json.encode({status="offline", ts=os.time()}))

        -- 连接server
        result = mqtt_client:connect()
        -- 如果连接server失败
        if not result then
            log.error("mqtt_client_main_task_func", "mqtt_client:connect error")
            goto EXCEPTION_PROC
        end


        -- 连接、断开连接、订阅、取消订阅、异常等各种事件的处理调度逻辑
        while true do
            -- 等待"MQTT_EVENT"消息
            msg = sys.waitMsg(TASK_NAME, "MQTT_EVENT")
            log.info("mqtt_client_main_task_func waitMsg", msg[2], msg[3], msg[4])

            -- connect连接结果
            -- msg[3]表示连接结果，true为连接成功，false为连接失败
            if msg[2] == "CONNECT" then
                -- mqtt连接成功
                if msg[3] then
                    log.info("mqtt_client_main_task_func", "connect success")
                    log.info("========================================")
                    log.info("DEVICE ONLINE:", mobile.imei())
                    log.info("========================================")
                    -- 通知mqtt sender数据发送应用模块的task，MQTT连接成功
                    sys.sendMsg(mqtt_sender.TASK_NAME, "MQTT_EVENT", "CONNECT_OK", mqtt_client)
                -- mqtt连接失败
                else
                    log.info("mqtt_client_main_task_func", "connect error")
                    -- 退出循环，发起重连
                    break
                end

            -- subscribe订阅结果
            -- msg[3]表示订阅结果，true为订阅成功，false为订阅失败
            elseif msg[2] == "SUBSCRIBE" then
                -- 订阅成功
                if msg[3] then
                    log.info("mqtt_client_main_task_func", "subscribe success", "qos: "..(msg[4] or "nil"))
                    log.info("========================================")
                    log.info("DEVICE SUBSCRIBED:", mobile.imei())
                    log.info("TOPIC:", "wendao/"..mobile.imei().."/{control,data/ack,ota}")
                    log.info("========================================")
                -- 订阅失败
                else
                    log.error("mqtt_client_main_task_func", "subscribe error", "code", msg[4])
                    -- 主动断开mqtt client连接
                    mqtt_client:disconnect()
                    -- 发送disconnect之后，此处延时1秒，给数据发送预留一点儿时间，发送到服务器；
                    -- 即使1秒的时间不足以发送给服务器也没关系；对服务器来说，mqtt客户端只是没有优雅的断开，不影响什么实质功能；
                    sys.wait(1000)
                    break
                end

            -- unsubscribe取消订阅成功
            elseif msg[2] == "UNSUBSCRIBE" then
                log.info("mqtt_client_main_task_func", "unsubscribe success")

            -- 需要主动关闭mqtt连接
            -- 用户需要主动关闭mqtt连接时，可以调用sys.sendMsg(TASK_NAME, "MQTT_EVENT", "CLOSE")
            elseif msg[2] == "CLOSE" then
                -- 主动断开mqtt client连接
                mqtt_client:disconnect()
                -- 发送disconnect之后，此处延时1秒，给数据发送预留一点儿时间，发送到服务器；
                -- 即使1秒的时间不足以发送给服务器也没关系；对服务器来说，mqtt客户端只是没有优雅的断开，不影响什么实质功能；
                sys.wait(1000)
                break

            -- 被动关闭了mqtt连接
            -- 被网络或者服务器断开了连接
            elseif msg[2] == "DISCONNECTED" then
                break

            -- 出现了其他异常
            elseif msg[2] == "ERROR" then
                break
            end
        end

        -- 出现异常
        ::EXCEPTION_PROC::

        -- 清空此task绑定的消息队列中的未处理的消息
        sys.cleanMsg(TASK_NAME)

        -- 通知mqtt sender数据发送应用模块的task，MQTT连接已经断开
        sys.sendMsg(mqtt_sender.TASK_NAME, "MQTT_EVENT", "DISCONNECTED")

        -- 如果存在mqtt client对象
        if mqtt_client then
            -- 关闭mqtt client，并且释放mqtt client对象
            mqtt_client:close()
            mqtt_client = nil
        end

        -- 5秒后跳转到循环体开始位置，自动发起重连
        sys.wait(5000)
    end
end

--创建并且启动一个task
--运行这个task的处理函数mqtt_client_main_task_func
sys.taskInitEx(mqtt_client_main_task_func, TASK_NAME)
