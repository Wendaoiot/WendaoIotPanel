--[[

@module  dio_test
@summary 四路数字输入(DI)和四路数字输出(DO)测试功能模块
@version 1.0

@usage
本文件为四路DI/DO测试功能模块，核心业务逻辑为：
1、初始化四路DI为输入模式(带上拉/下拉)；
2、初始化四路DO为输出模式；
3、启动两个task：
   - DI读取task: 每100ms读取一次DI电平并打印；
   - DO翻转task: 每1秒翻转一次四路DO电平；

硬件连接说明(对应原理图)：
+----------+-----------+----------+----------------+-----------------------+
| 类型     | 通道      | GPIO     | 说明           | 备注                  |
+----------+-----------+----------+----------------+-----------------------+
| DI输入   | DI1       | GPIO34   | 数字输入1      |                默认低   |
| DI输入   | DI2       | GPIO3    | 数字输入2      |                默认低   |
| DI输入   | DI3       | GPIO6    | 数字输入3      |                默认低   |
| DI输入   | DI4       | GPIO7    | 数字输入4      |                默认低   |
| DO输出   | DO1       | GPIO22   | 数字输出1      | SOD882保护二极管      |
| DO输出   | DO2       | GPIO24   | 数字输出2      | SOD882保护二极管      |
| DO输出   | DO3       | GPIO1    | 数字输出3      | SOD882保护二极管      |
| DO输出   | DO4       | GPIO2    | 数字输出4      | SOD882保护二极管      |
+----------+-----------+----------+----------------+-----------------------+

DI说明：
- 硬件带10k下拉电阻，无外部输入时DI默认为低电平(0)
- 外部输入高电平(3.3V)时DI为高电平(1)
- 通过gpio.setup()配置为输入模式即可

DO说明：
- 配置为推挽输出模式
- 默认初始化输出低电平(0)
- 通过gpio.toggle()或gpio.set()控制输出状态
]]


-- 四路DI配置表
-- mode: "pulldown"=下拉(默认低), "pullup"=上拉(默认高), "float"=浮空
local DI_CHANNELS = {
    {id = 1, name = "DI1", gpio = 34, mode = "pulldown"},
    {id = 2, name = "DI2", gpio = 3,  mode = "pulldown"},
    {id = 3, name = "DI3", gpio = 6,  mode = "pulldown"},
    {id = 4, name = "DI4", gpio = 7,  mode = "pulldown"},
}

-- 四路DO配置表
-- init_level: 初始输出电平, 0=低, 1=高
local DO_CHANNELS = {
    {id = 1, name = "DO1", gpio = 22, init_level = 0},
    {id = 2, name = "DO2", gpio = 24, init_level = 0},
    {id = 3, name = "DO3", gpio = 1,  init_level = 0},
    {id = 4, name = "DO4", gpio = 2,  init_level = 0},
}

-- DI读取周期(毫秒)
local DI_READ_INTERVAL_MS = 100
-- DO翻转周期(毫秒)
local DO_TOGGLE_INTERVAL_MS = 1000


-- 初始化四路DI
for _, ch in ipairs(DI_CHANNELS) do
    local result = gpio.setup(ch.gpio, ch.mode)
    log.info("dio_test.di", ch.name, "gpio="..ch.gpio, "mode="..ch.mode, result and "ok" or "fail")
end

-- 初始化四路DO
for _, ch in ipairs(DO_CHANNELS) do
    local result = gpio.setup(ch.gpio, ch.init_level)
    log.info("dio_test.do", ch.name, "gpio="..ch.gpio, "init="..ch.init_level, result and "ok" or "fail")
end


-- 挂起标志，休眠期间停止DI读取和DO翻转(配合sleep_test模块的低功耗休眠)
local suspended = false

sys.subscribe("APP_SUSPEND", function()
    suspended = true
end)

sys.subscribe("APP_RESUME", function()
    suspended = false
end)


-- DI读取task
local function di_read_task_func()
    sys.wait(500)

    while true do
        if suspended then
            -- 已挂起: 阻塞等待恢复消息，task完全阻塞有利于系统进入休眠
            sys.waitUntil("APP_RESUME")
        else
            local status_str = ""
            for _, ch in ipairs(DI_CHANNELS) do
                local val = gpio.get(ch.gpio)
                local level_str = val == 1 and "HIGH" or "LOW"
                status_str = status_str .. string.format(" %s=%s", ch.name, level_str)
            end

            log.info("dio_test.di_read", status_str)
            sys.wait(DI_READ_INTERVAL_MS)
        end
    end
end


-- DO翻转task
local function do_toggle_task_func()
    sys.wait(1000)

    while true do
        if suspended then
            -- 已挂起: DO全部输出低电平省电，阻塞等待恢复消息
            for _, ch in ipairs(DO_CHANNELS) do
                gpio.set(ch.gpio, 0)
            end
            log.info("dio_test", "suspend, all DO -> LOW")
            sys.waitUntil("APP_RESUME")
        else
            for _, ch in ipairs(DO_CHANNELS) do
                gpio.toggle(ch.gpio)
                local val = gpio.get(ch.gpio)
                local level_str = val == 1 and "HIGH" or "LOW"
                log.info("dio_test.do_toggle", ch.name, "gpio="..ch.gpio, "-> "..level_str)
            end

            sys.wait(DO_TOGGLE_INTERVAL_MS)
        end
    end
end


log.info("dio_test", "init done, DI="..#DI_CHANNELS.."ch, DO="..#DO_CHANNELS.."ch")

-- 启动DI读取和DO翻转task
sys.taskInit(di_read_task_func)
sys.taskInit(do_toggle_task_func)
