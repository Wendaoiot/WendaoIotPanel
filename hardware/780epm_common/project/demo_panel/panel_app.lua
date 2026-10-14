--[[
@module  panel_app
@summary wendaoiot 面板(云端平台)连接协议：周期上报面板数据 + 控制指令应答
@version 1.0

@usage
本示例跑通 wendaoiot 面板的 MQTT 数据通道（无真实传感器，tags 为让数据点"动起来"的演示值）：

  上行（设备 → 平台）
    wendao/<IMEI>/data          周期面板数据，见下方 payload 格式 (qos=1)
    wendao/<IMEI>/control/ack   控制指令应答 (qos=0)
    wendao/<IMEI>/status        遗嘱，异常断线时 {"status":"offline"}（mqtt_wendao 内置）
  下行（平台 → 设备）
    wendao/<IMEI>/control       控制指令，payload 如 {"id":"...","tags":{"relay1":1}}

实际项目接入时：把 report_panel_data() 里的演示 tags 换成您的真实采集值
（可结合 demo_4_20ma / demo_0_5v / demo_rs485 的采集结果），
在 control 处理的 TODO 处加上实际的继电器/DO 控制动作即可。
]]

local mqtt_wendao = require "mqtt_wendao"

-- ===================== 配置区 =====================
local REPORT_PERIOD_MS = 2000   -- 面板数据上报周期（毫秒）
-- ===================================================

local first_ts = os.time()   -- 本次开机时间戳（平台侧统计在线时长用）
local seq = 0                -- 帧计数，用来生成明显在变化的演示数据

-- 上报一帧面板数据
-- payload 与产品数据格式一致：{"id","ts","version","first_ts","tags":{...}}
local function report_panel_data()
    seq = seq + 1
    local payload = json.encode({
        id = "msg_" .. os.time() .. "_" .. seq,
        ts = os.time(),
        version = VERSION or "demo",
        first_ts = first_ts,
        tags = {
            -- 下列四个量为演示值（随帧变化，便于在面板上看到数据点刷新）；接入项目时替换为真实采集值
            temperature = 25 + math.sin(seq / 5) * 5,  -- 演示：在 20~30℃ 之间周期变化
            humidity    = 60 + seq % 20,               -- 演示：60~79 %RH
            voltage     = 24 + (seq % 10) / 10,        -- 演示：24.0~24.9 V
            current     = 1.0 + (seq % 100) / 100      -- 演示：1.00~1.99 A
        }
    })
    mqtt_wendao.pub("data", payload, 1)
    log.info("panel_app", "report:", payload)
end

-- 周期上报任务（mqtt_wendao 自带断网暂存队列，未连上池数据会在连上后发出）
sys.taskInit(function()
    sys.wait(2000)   -- 等网络/连接建立
    while true do
        report_panel_data()
        sys.wait(REPORT_PERIOD_MS)
    end
end)

-- 订阅平台下发的控制指令
mqtt_wendao.subscribe("control")
sys.subscribe("WENDAO_RECV", function(suffix, payload)
    if suffix == "control" then
        log.info("panel_app", "recv control:", payload)

        local ok, cmd = pcall(json.decode, payload)
        if ok and cmd and cmd.id then
            -- 执行 / 展示控制项（pid 例如 cmd.tags = {"relay1"=1, "relay2"=0}）
            -- TODO: 在此结合您的硬件执行控制面板继电器、DO 输出等动作
            if cmd.tags then
                for key, value in pairs(cmd.tags) do
                    log.info("panel_app", "set", key, "=", value)
                end
            end

            -- 回复控制应答：id 与指令一致，code=0 表示成功
            mqtt_wendao.pub("control/ack", json.encode({ id = cmd.id, code = 0, msg = "ok" }), 0)
        else
            log.error("panel_app", "invalid control payload:", payload)
        end
    end
end)

log.info("panel_app", "ready: up=<IMEI>/data & control/ack, down=<IMEI>/control")
