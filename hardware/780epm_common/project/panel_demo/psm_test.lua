--[[

@module  psm_test
@summary PSM+模式MQTT短连接低功耗测试功能模块(断网压榨功耗)
@version 1.0

@usage
本文件为PSM+模式下的MQTT短连接测试功能模块(参照官方lowpower demo的prj_3_mqtt_short)，
核心业务逻辑为：
1、开机后打印唤醒原因(pm.lastReson)，判断是上电开机/定时器唤醒/引脚唤醒；
2、等待网络注册+MQTT连接成功，发送一次数据到MQTT服务器(短连接生命周期内唯一一次上报)；
3、发送完成后主动断开MQTT连接，并通知mqtt_main停止自动重连(MQTT_STOP_REQ)，
   防止MQTT重连注网活动阻止系统进入PSM+模式；
4、挂起所有业务模块(APP_SUSPEND)，GPIO低功耗配置；
5、配置深度休眠定时器(dtimer)，然后pm.power(pm.WORK_MODE, 3)进入PSM+模式；
6、PSM+模式下完全断网，功耗约3uA左右(3~10uA属正常值)；
7、dtimer超时(或WAKEUP引脚/PWR_KEY/UART1_RXD中断)唤醒后软件系统直接重启，
   重启后重新执行本流程(注网→连接→上报→断开→PSM+休眠)；

PSM+与低功耗模式1(WORK_MODE 1)的区别：
- WORK_MODE 1: 4G保持在线(~1.5mA)，可MQTT远程唤醒，休眠中系统继续运行(唤醒不重启)
- WORK_MODE 3(PSM+): 完全断网(~3uA)，无法远程唤醒，唤醒即重启；
  唤醒方式仅限: dtimer定时 / WAKEUP0~5引脚中断 / PWR_KEY / VBUS / UART1_RXD中断

重要注意事项：
1、dtimer周期不要小于80秒(内核最长需要75秒才能确保进入PSM+模式)；
2、休眠期间硬件看门狗(GPIO27)停止喂狗并锁定低电平！硬件看门狗超时时间必须大于
   唤醒周期，否则休眠期间触发硬件复位；建议测试时断开看门狗电路；
3、network_watchdog看门狗(20分钟)大于默认唤醒周期(5分钟)，且唤醒重启后task重新
   计时，唤醒后MQTT连接成功即喂狗，不会误重启；
4、PSM+下USB自动关闭(2025.03后内核固件WORK_MODE 3自动执行pm.power(pm.USB, false))，
   开机和唤醒重启后USB自动恢复，可正常看日志；
]]


-------------------- 配置参数 --------------------
-- PSM+唤醒周期(毫秒)，默认5分钟
-- 官方要求: 不要小于80秒；量产建议几十分钟以上才有省电意义
local PSM_WAKEUP_MS = 5 * 60 * 1000

-- 等待网络注册超时(毫秒)
local NET_WAIT_MS = 60 * 1000

-- 等待MQTT连接建立的延时(毫秒)，IP_READY后注网+MQTT连接需要时间
local MQTT_CONNECT_WAIT_MS = 15 * 1000

-- 等待MQTT数据发送结果的超时(毫秒)
local SEND_RSP_TIMEOUT_MS = 15 * 1000

-- 断开MQTT后等待协议栈清理的时间(毫秒)
local MQTT_CLOSE_WAIT_MS = 3000

-- APP_SUSPEND后等待各业务模块完成挂起清理的时间(毫秒)
local SUSPEND_SETTLE_MS = 3000


-------------------- mqtt_main的task名(用于发送CLOSE消息) --------------------
local mqtt_sender = require "mqtt_sender"
local MQTT_MAIN_TASK = mqtt_sender.TASK_NAME_PREFIX.."main"


-------------------- 唤醒原因 --------------------
-- pm.lastReson()返回值含义(参照官方drv_psm.lua注释)
local WAKEUP_REASON_MAP = {
    [0] = "poweron(上电开机)",
    [1] = "dtimer(深度休眠定时器唤醒)",
    [2] = "wakeup(WAKEUP引脚中断唤醒)",
    [3] = "uart1_rxd(UART1_RXD数据唤醒)",
    [4] = "reset(Reset重启/软件重启/看门狗重启)",
    [5] = "pwrkey(开机键唤醒)",
    [6] = "chg_det(CHG_DET唤醒)",
}

local function log_wakeup_reason()
    local reason = pm.lastReson()
    local desc = WAKEUP_REASON_MAP[reason] or ("unknown("..tostring(reason)..")")
    -- pm.dtimerWkId(): >=0 表示本次是深度休眠定时器唤醒(值为定时器ID)，-1表示不是
    local wk_id = pm.dtimerWkId and pm.dtimerWkId() or "N/A"
    log.info("psm_test", "唤醒原因:", reason, desc, "dtimerWkId=", wk_id)
    return reason, desc
end


-------------------- GPIO进入PSM+低功耗配置 --------------------
-- 与sleep_test.lua的gpio_enter_sleep保持一致的策略
local function gpio_enter_psm()
    -- 关闭两路升压电路使能，切断RS485_5V等外围供电
    gpio.setup(37, 0)
    gpio.set(37, 0)
    gpio.setup(38, 0)
    gpio.set(38, 0)
    log.info("psm_test.gpio", "EN1(37)/EN2(38)升压已关闭")

    -- 硬件看门狗喂狗脚锁定低电平(PSM+下AGPIO电平保持不变，重启后也不会变)
    gpio.setup(27, 0)
    gpio.set(27, 0)

    -- 关闭UART2(RS485)，消除RX悬空噪声
    uart.close(2)
    -- DIR脚(GPIO33)输出低电平，避免向断电的收发器灌电流
    gpio.setup(33, 0)
    gpio.set(33, 0)
    log.info("psm_test.gpio", "UART2已关闭, GPIO33(DIR)锁定低电平")

    -- 关闭Vref参考电压管脚(GPIO23)，降低功耗(本工程未使用Vref)
    gpio.setup(23, nil, gpio.PULLDOWN)

    -- WAKEUP1(VBUS)/WAKEUP2(SIM_DET)配置下拉防止漏电
    gpio.setup(gpio.WAKEUP1, nil, gpio.PULLDOWN)
    gpio.setup(gpio.WAKEUP2, nil, gpio.PULLDOWN)

    -- 注意: WAKEUP5=GPIO22与本工程DO1复用，由dio_test模块管理，此处不配置
end


-------------------- PSM+短连接主task --------------------
local function psm_test_main()
    -- 1、打印唤醒原因
    local reason, reason_desc = log_wakeup_reason()

    -- 2、等待网络注册
    local net_ok = sys.waitUntil("IP_READY", NET_WAIT_MS)
    log.info("psm_test", "IP_READY:", net_ok == true)
    if not net_ok then
        -- 网络注册失败也要走完流程进PSM+，等dtimer唤醒重启重试
        log.error("psm_test", "wait IP_READY timeout")
    end

    -- 3、等待MQTT连接建立(发送的数据会进入mqtt_sender队列，连接成功后自动发出)
    sys.wait(MQTT_CONNECT_WAIT_MS)

    -- 4、发送一次数据(短连接生命周期内唯一一次上报，带回调获取发送结果)
    local send_result = false
    local psm_payload = string.format(
        '{"wakeup_reason":%d,"wakeup_desc":"%s","ts":%d}',
        reason, reason_desc, os.time())
    sys.publish("SEND_DATA_REQ", "psm_test",
        "wendao/"..mobile.imei().."/psm/up", psm_payload, 0,
        {func = function(result)
            send_result = result
            sys.publish("PSM_SEND_RSP")
        end})

    -- 5、等待发送结果
    local rsp_ok = sys.waitUntil("PSM_SEND_RSP", SEND_RSP_TIMEOUT_MS)
    log.info("psm_test", "send data:", rsp_ok == true, send_result)

    -- 6、主动断开MQTT短连接
    sys.sendMsg(MQTT_MAIN_TASK, "MQTT_EVENT", "CLOSE")
    sys.wait(MQTT_CLOSE_WAIT_MS)

    -- 7、通知mqtt_main停止自动重连(防止注网活动阻止进入PSM+)
    sys.publish("MQTT_STOP_REQ")
    log.info("psm_test", "MQTT已断开且停止自动重连")

    -- 8、挂起所有业务模块(停止周期性任务)
    sys.publish("APP_SUSPEND")
    sys.wait(SUSPEND_SETTLE_MS)

    -- 9、GPIO低功耗配置
    gpio_enter_psm()

    -- 10、配置深度休眠定时器，进入PSM+模式
    -- 注意: dtimerStart必须在pm.power(pm.WORK_MODE, 3)之前调用
    -- Air780EPM dtimer说明: ID 0~5可用；ID 0/1最大2.5小时，ID 2~5最大740小时
    local dtimer_ok = pm.dtimerStart(0, PSM_WAKEUP_MS)
    -- dtimerCheck诊断: 确认定时器真正在运行以及剩余时间
    local check_run, check_left = pm.dtimerCheck(0)
    log.info("psm_test", "dtimerStart id=0 ms="..PSM_WAKEUP_MS,
        "start_result="..tostring(dtimer_ok),
        "check_running="..tostring(check_run),
        "check_left_ms="..tostring(check_left))
    if not dtimer_ok or not check_run then
        log.error("psm_test", "dtimer id=0 异常! 尝试ID=2(最大740小时)")
        pm.dtimerStop(0)
        dtimer_ok = pm.dtimerStart(2, PSM_WAKEUP_MS)
        check_run, check_left = pm.dtimerCheck(2)
        log.info("psm_test", "dtimerStart id=2",
            "start_result="..tostring(dtimer_ok),
            "check_running="..tostring(check_run),
            "check_left_ms="..tostring(check_left))
    end

    log.info("psm_test", "========== 进入PSM+模式 ==========")
    log.info("psm_test", "唤醒周期:", PSM_WAKEUP_MS/1000, "秒")
    log.info("psm_test", "断网休眠，预期功耗~3uA；唤醒后系统重启重新走本流程")
    pm.power(pm.WORK_MODE, 3)

    -- 诊断: WORK_MODE 3设置后立即复查dtimer是否仍在运行
    -- 如果此处check_running=false，说明进入PSM+的过程中dtimer被清除(固件行为)，这就是不唤醒的根因
    sys.wait(1000)
    local run_after, left_after = pm.dtimerCheck(0)
    log.info("psm_test", "[诊断] WORK_MODE3后 dtimer0 running="..tostring(run_after),
        "left_ms="..tostring(left_after))

    -- 11、80秒兜底: 正常情况下执行不到这里(成功进入PSM+后RAM掉电)
    -- 内核最长75秒确保进入PSM+，若80秒仍未进入则重启重试
    sys.wait(80000)
    log.error("psm_test", "进入PSM+失败，重启重试")
    rtos.reboot()
end

log.info("psm_test", "init done, PSM+短连接模式: 上报一次→断网→PSM+→dtimer唤醒重启循环")

-- 创建并且启动PSM+短连接测试task
sys.taskInit(psm_test_main)
