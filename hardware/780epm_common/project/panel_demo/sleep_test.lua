--[[

@module  sleep_test
@summary 低功耗休眠测试功能模块(联网休眠+仅MQTT远程唤醒)
@version 1.2

@usage
本文件为低功耗休眠测试功能模块，核心业务逻辑为：
1、等待网络和MQTT连接就绪后，进入低功耗休眠模式(WORK_MODE 1)，4G/MQTT保持在线；
2、休眠期间仅支持远程唤醒(定时唤醒已关闭)：
   - 远程唤醒: MQTT服务器下发任意数据自动唤醒(4G在线收到数据自动唤醒系统)；
   - 休眠期间唯一的周期性网络活动是MQTT keepalive(600s)保活心跳；
3、静默功耗模式(RESUME_BUSINESS=false，默认)：
   休眠后所有业务模块(喂狗/RS485/ADC/DI-DO/MQTT定时上报/UART2)保持关闭不再恢复，
   只保留4G网络连接+MQTT远程唤醒+唤醒状态上报，功耗最低；
4、功能测试模式(RESUME_BUSINESS=true)：
   每次唤醒后通过"APP_RESUME"恢复所有业务模块运行，活动窗口结束后重新休眠挂起；

功耗模式说明(参考官方demo):
- WORK_MODE 0: 常规模式，功耗约6mA
- WORK_MODE 1: 低功耗模式，休眠电流约1~1.5mA，4G保持在线，可远程唤醒(本模块使用)
- WORK_MODE 3: PSM+模式，功耗约3uA，完全断网，需重新注网

功耗实测分析(10mA问题的根因):
- 唤醒窗口占空比过大: 40s周期中10s处于常规模式(约40-50mA)，平均电流≈11.6mA
  解决: 活动窗口缩短到3s + 静默模式不再恢复业务
- USB常开会阻止系统进入深度休眠: 测功耗必须DEBUG_MODE=false
- UART2_RX悬空噪声: RS485收发器断电后RO脚悬空，噪声中断反复唤醒系统
  解决: 休眠时uart.close(2)并锁定GPIO33为低电平

重要注意事项：
1、DEBUG_MODE=false(默认): 测真实功耗必须用此配置，USB自动关闭；
   需要看日志时改true(功耗会增加，且无法进入最深度休眠)；
2、休眠期间硬件看门狗(GPIO27)停止喂狗并锁定低电平！硬件看门狗超时时间必须大于
   休眠周期(40s)，否则休眠期间会触发硬件复位，测功耗时建议先断开看门狗电路；
3、休眠时EN1(GPIO37)/EN2(GPIO38)拉低关闭两路升压电路(RS485_5V等)；
]]


-------------------- 配置参数 --------------------
-- 调试模式: false=关闭USB测真实功耗(默认), true=保持USB开启可看日志
local DEBUG_MODE = false

-- 静默功耗模式: false=休眠后不恢复业务模块，只保留网络+唤醒(功耗最低，默认)
--              true=每次唤醒后恢复业务模块运行(功能测试用)
local RESUME_BUSINESS = false

-- 唤醒后活动窗口(毫秒)
-- 静默模式下仅需完成唤醒状态MQTT上报，3秒足够
-- 注意: 此窗口处于常规模式(约40-50mA)，窗口越长平均功耗越高
local ACTIVE_WINDOW_MS = 3 * 1000

-- 开机后延时(毫秒)，等待网络注册+MQTT连接建立完成后再进入休眠
local BOOT_DELAY_MS = 20 * 1000

-- 挂起等待时间(毫秒)，发布APP_SUSPEND后等待各业务模块完成挂起清理
local SUSPEND_SETTLE_MS = 3000

-------------------- 唤醒状态记录 --------------------
local is_sleeping = false       -- 当前是否处于休眠状态
local business_suspended = false -- 业务模块当前是否处于挂起状态
local wakeup_count = 0          -- 累计唤醒次数
local wakeup_source = "boot"    -- 最近一次唤醒来源: boot/timer/remote


-------------------- 休眠GPIO配置 --------------------
-- 进入休眠前的GPIO低功耗配置
local function gpio_enter_sleep()
    -- 关闭两路升压电路使能，切断RS485_5V等外围供电
    gpio.setup(37, 0)
    gpio.set(37, 0)
    gpio.setup(38, 0)
    gpio.set(38, 0)
    log.info("sleep_test.gpio", "EN1(37)/EN2(38)升压已关闭")

    -- 硬件看门狗喂狗脚锁定低电平，避免停在高电平向外围电路灌电流
    gpio.setup(27, 0)
    gpio.set(27, 0)

    -- 关闭UART2(RS485)，消除RX悬空噪声中断反复唤醒系统的问题
    -- EN2断电后RS485收发器RO脚悬空，噪声会导致UART2中断不断触发
    uart.close(2)
    -- DIR脚(GPIO33)输出低电平，避免向断电的收发器灌电流(寄生供电)
    gpio.setup(33, 0)
    gpio.set(33, 0)
    log.info("sleep_test.gpio", "UART2已关闭, GPIO33(DIR)锁定低电平")

    -- 关闭Vref参考电压管脚(GPIO23)，降低功耗(本工程未使用Vref)
    gpio.setup(23, nil, gpio.PULLDOWN)

    -- WAKEUP1(VBUS)/WAKEUP2(SIM_DET)配置下拉防止漏电
    gpio.setup(gpio.WAKEUP1, nil, gpio.PULLDOWN)
    gpio.setup(gpio.WAKEUP2, nil, gpio.PULLDOWN)

    -- 注意: WAKEUP5=GPIO22与本工程DO1复用，由dio_test模块管理，此处不配置
end

-- 唤醒后的GPIO恢复配置(仅RESUME_BUSINESS=true时调用)
local function gpio_exit_sleep()
    -- 恢复两路升压电路使能
    gpio.setup(37, 0)
    gpio.set(37, 1)
    gpio.setup(38, 0)
    gpio.set(38, 1)
    log.info("sleep_test.gpio", "EN1(37)/EN2(38)升压已恢复")

    -- 恢复Vref参考电压输出
    gpio.setup(23, 1)

    -- UART2由rs485_test模块在APP_RESUME消息中重新初始化
end


-------------------- 进入/退出休眠 --------------------
local function enter_sleep()
    if is_sleeping then
        return
    end

    is_sleeping = true
    log.info("sleep_test", "========== 进入休眠 ==========")
    log.info("sleep_test", "唤醒次数:", wakeup_count, "上次唤醒来源:", wakeup_source)

    -- 通知所有业务模块挂起(停止周期性任务，否则无法真正休眠)
    if not business_suspended then
        sys.publish("APP_SUSPEND")
        -- 等待各模块完成挂起清理(ADC关通道、DO拉低等)
        sys.wait(SUSPEND_SETTLE_MS)
        business_suspended = true
    end

    -- GPIO低功耗配置
    gpio_enter_sleep()

    -- 非调试模式关闭USB: USB常开会阻止系统进入深度休眠！
    if not DEBUG_MODE then
        pm.power(pm.USB, false)
    end

    -- 进入低功耗模式MODE 1: 4G保持在线，可远程唤醒
    -- 注意: 执行后系统在所有task阻塞时自动休眠，task唤醒时自动恢复运行
    pm.power(pm.WORK_MODE, 1)
    log.info("sleep_test", "已进入WORK_MODE 1，4G在线，USB已关闭，等待MQTT远程唤醒...")
end

local function exit_sleep(source)
    if not is_sleeping then
        return
    end

    -- 首先退出低功耗模式
    pm.power(pm.WORK_MODE, 0)

    is_sleeping = false
    wakeup_count = wakeup_count + 1
    wakeup_source = source or "unknown"

    -- 参照motor_ctrl: 唤醒后无条件打开USB查看日志
    -- 非DEBUG_MODE时USB在休眠期间被关闭，唤醒后必须重新打开才能看日志
    if not DEBUG_MODE then
        pm.power(pm.USB, true)
        -- 首次唤醒USB日志口刚重新枚举，等待稳定后再输出关键唤醒日志
        -- 后续唤醒等待500ms即可
        if wakeup_count == 1 then
            sys.wait(3000)
        else
            sys.wait(500)
        end
    end

    log.info("sleep_test", "========== 从休眠唤醒 ==========")
    log.info("sleep_test", "唤醒来源:", wakeup_source, "累计唤醒次数:", wakeup_count)

    if RESUME_BUSINESS then
        -- GPIO恢复
        gpio_exit_sleep()

        -- 通知所有业务模块恢复运行
        business_suspended = false
        sys.publish("APP_RESUME")
    else
        -- 静默功耗模式: 业务保持关闭，仅网络+上报唤醒状态
        log.info("sleep_test", "静默模式: 业务保持挂起，仅网络+唤醒")
    end

    -- 喂网络环境看门狗(防止休眠期间看门狗超时复位)
    sys.publish("FEED_NETWORK_WATCHDOG")

    -- 通过MQTT上报唤醒状态(仅此一次上报，网络连接保持)
    local wakeup_payload = string.format(
        '{"wakeup_source":"%s","wakeup_count":%d}',
        wakeup_source, wakeup_count)
    sys.publish("SEND_DATA_REQ", "sleep_wakeup",
        mobile.imei().."/sleep/up", wakeup_payload, 0)
end


-------------------- 远程唤醒处理 --------------------
-- MQTT服务器下发任意数据(publish到非控制topic)时，mqtt_receiver会发布RECV_DATA_FROM_SERVER消息
-- 休眠期间4G在线，收到数据自动唤醒系统，此处识别唤醒来源
sys.subscribe("RECV_DATA_FROM_SERVER", function(prefix, topic, payload)
    if is_sleeping then
        log.info("sleep_test", "检测到远程下发数据，触发远程唤醒")
        sys.publish("WAKEUP_EVENT", "remote")
    end
end)


-------------------- 主控制task --------------------
local function sleep_test_main()
    -- 等待网络注册
    sys.waitUntil("IP_READY", 60000)
    -- 等待MQTT连接建立
    sys.wait(BOOT_DELAY_MS)

    log.info("sleep_test", "========== 休眠功耗测试启动 ==========")
    log.info("sleep_test", "调试模式:", DEBUG_MODE and "开(USB保持,无法测真实功耗)" or "关(USB断开)")
    log.info("sleep_test", "业务恢复:", RESUME_BUSINESS and "开(唤醒后恢复业务)" or "关(静默功耗模式)")
    log.info("sleep_test", "定时唤醒: 已关闭，仅MQTT远程唤醒")
    log.info("sleep_test", "唤醒活动窗口:", ACTIVE_WINDOW_MS / 1000, "秒")
    log.info("sleep_test", "远程唤醒: MQTT下发任意非控制topic数据")
    log.info("sleep_test", "MQTT保活: keepalive=600s(唯一周期性网络活动)")

    while true do
        -- 进入休眠
        enter_sleep()

        -- 无限期阻塞等待远程唤醒事件(定时唤醒已关闭)
        -- 远程唤醒时RECV_DATA_FROM_SERVER订阅会发布WAKEUP_EVENT消息
        local _, source = sys.waitUntil("WAKEUP_EVENT")

        -- 远程唤醒
        exit_sleep(source or "remote")

        -- 活动窗口: 处理远程下发数据+完成唤醒上报；结束后重新休眠
        sys.wait(ACTIVE_WINDOW_MS)
    end
end

log.info("sleep_test", "init done, work_mode=1联网休眠, 仅MQTT远程唤醒(无定时唤醒)")

-- 创建并且启动休眠主控制task
sys.taskInit(sleep_test_main)
