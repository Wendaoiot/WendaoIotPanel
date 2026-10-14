--[[
@module  main
@summary 4-20mA 电流环采集上报 demo —— 程序入口
@version 1.0

@usage
功能：采集 4-20mA 电流环输入（ADC1 / 模组 PIN96），换算成电流(mA)和量程百分比，
      每 2 秒一次以 JSON 上报到 wendao/<IMEI>/data。

文件说明：
  main.lua         本文件，入口（上电初始化 + 加载模块）
  mqtt_wendao.lua  wendaoiot 平台 MQTT 连接（通用，几个 demo 共用同一文件）
  adc_4_20ma.lua   4-20mA 采集与换算业务
]]

-- Luatools 烧录和版本管理需要这两个变量
PROJECT = "DEMO_4_20MA"
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
-- 加载 4-20mA 采集业务模块
require "adc_4_20ma"

-- 用户代码已结束---------------------------------------------
-- 结尾总是这一句
sys.run()
-- sys.run()之后不要加任何语句!!!!!因为添加的任何语句都不会被执行
