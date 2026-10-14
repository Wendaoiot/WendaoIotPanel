--[[
@module  main
@summary wendaoiot 面板(平台)连接 demo —— 程序入口
@version 1.0

@usage
功能：连接 wendaoiot 面板（云端管理平台）并跑通完整数据通道
  - 每 2 秒向 wendao/<IMEI>/data 上报一帧面板数据（JSON）
  - 订阅 wendao/<IMEI>/control 控制指令，回复 wendao/<IMEI>/control/ack
  - 设备异常断线时平台收到遗嘱 wendao/<IMEI>/status（offline）

文件说明：
  main.lua        本文件，入口（上电初始化 + 加载模块）
  mqtt_wendao.lua wendaoiot 平台 MQTT 连接（几组 demo 共用同一文件）
  panel_app.lua   面板协议业务（数据上报 + 控制应答）
]]

-- Luatools 烧录和版本管理需要这两个变量
PROJECT = "DEMO_PANEL"
VERSION = "001.999.001"

log.info("main", PROJECT, VERSION)

-- 打印设备信息（IMEI 就是 MQTT 主题里的设备编号，平台侧用它识别设备）
sys.timerStart(function()
    log.info("main", "IMEI", mobile.imei())
    log.info("main", "ICCID", mobile.iccid(), "CSQ", mobile.csq(), "NetStatus", mobile.status())
end, 3000)

-- 【38盒子 V3.1 板载电路】两路升压使能：EN1=GPIO37（RS485_5V 供电）、EN2=GPIO38
-- 自己做的板子如果没有这两路电源，删除下面两行即可
gpio.setup(37, 1)
gpio.setup(38, 1)

-- 【38盒子 V3.1 板载电路】硬件看门狗喂狗：WDT 芯片接 GPIO27，
-- 必须每 1 秒翻转一次电平，超时未翻转看门狗会复位整机。
-- 自己做的板子如果没有外部看门狗芯片，删除下面这段即可
sys.taskInit(function()
    gpio.setup(27, 0)
    while true do
        gpio.toggle(27)
        sys.wait(1000)
    end
end)

-- 加载 wendaoiot 平台 MQTT 连接模块
require "mqtt_wendao"
-- 加载面板协议业务模块
require "panel_app"

-- 用户代码已结束---------------------------------------------
-- 结尾总是这一句
sys.run()
-- sys.run()之后不要加任何语句!!!!!因为添加的任何语句都不会被执行
