--[[

@module  adc_test
@summary 四路ADC采样测试功能模块
@version 1.0

@usage
本文件为四路ADC采样测试功能模块，核心业务逻辑为：
1、初始化四路ADC通道(ADC0/ADC1/ADC2/ADC3)；
2、拉低分压电阻使能GPIO(4/20/28/36)使能分压电路；
3、启动一个task，每2秒对四路ADC进行一次采样；
4、根据分压电阻(150k上+53k下)换算出实际输入电压并打印日志；

硬件连接说明(对应原理图)：
+----------+-------------+----------------------+-----------+-------------+--------------+
| 通道     | ADC通道号   | 输入信号             | 模组PIN   | 使能GPIO    | 分压方案     |
+----------+-------------+----------------------+-----------+-------------+--------------+
| ADC0     | 0           | VBAT电压检测         | PIN9      | GPIO36      | 150k+53k     |
| ADC1     | 1           | 4-20mA电流输入       | PIN96     | GPIO20      | 150k+53k     |
| ADC2     | 2           | 0~5V_INPUT1电压输入  | PIN77     | GPIO4       | 150k+53k     |
| ADC3     | 3           | 0~5V_INPUT2电压输入  | PIN76     | GPIO28      | 150k+53k     |
+----------+-------------+----------------------+-----------+-------------+--------------+

分压电阻使能说明：
- GPIO(4/20/28/36) 控制分压电阻下端是否接地
- 推挽输出低电平(0V) → 分压电阻下端接地 → 形成标准分压 → ADC正常采样
- 推挽输出高电平     → 分压电阻下端悬空 → 分压回路断开 → 低功耗(不采样)

分压电阻换算说明：
- 上拉电阻 R上 = 150kΩ，下拉电阻 R下 = 53kΩ
- ADC采样电压 = 输入电压 × (R下 / (R上 + R下))
- 即 Vadc = Vin × (53 / (150+53)) = Vin × 53/203 ≈ Vin × 0.2611
- 反推输入电压: Vin = Vadc × (203/53) ≈ Vadc × 3.8302
- AUXADC有效输入范围0~1.6V，外接分压后可测最大输入 ≈ 1.6 × 3.8302 ≈ 6.13V
- 注意: 使能分压后ADC读到的是分压后的小电压，必须乘以系数才是实际输入电压

4-20mA通道(ADC1)特殊说明：
- 4-20mA电流经过采样电阻R18转换成电压后再经150k+53k分压进入ADC1
- 如果R18=250Ω(标准采样电阻)：
    4mA  → 1V  → 分压后ADC采样≈0.261V
    20mA → 5V  → 分压后ADC采样≈1.305V(在1.6V量程内)
- 电流换算: I = (Vadc × EXT_DIV_SCALE) / R18
]]


-- 分压电阻参数（单位: kΩ）
local R_UP = 150      -- 上拉分压电阻
local R_DOWN = 53     -- 下拉分压电阻
-- 外接分压电压换算系数: Vin = Vadc × (R_UP + R_DOWN) / R_DOWN
local EXT_DIV_SCALE = (R_UP + R_DOWN) / R_DOWN  -- ≈ 3.8302

-- ADC采样周期(毫秒)
local SAMPLE_INTERVAL_MS = 2000

-- 四通道配置表
-- gpio:    分压电阻下端接地控制脚，推挽输出低电平=使能分压
-- pin:     ADC对应的模组模拟引脚号
-- scale:   电压换算系数，Vin = vadc_mv × scale (使能分压后需乘EXT_DIV_SCALE)
-- offset:  电压校正偏移量(mV)，读数偏低填正值，读数偏高填负值
--          实测读数比实际电压低约200mV，故默认+200
local ADC_CHANNELS = {
    {id = 0, name = "ADC0_VBAT",    desc = "VBAT电压检测",   pin = 9,  gpio = 36, scale = EXT_DIV_SCALE, offset = 200},
    {id = 1, name = "ADC1_4_20mA",  desc = "4-20mA电流输入", pin = 96, gpio = 20, scale = EXT_DIV_SCALE, offset = 200},
    {id = 2, name = "ADC2_5V_IN1",  desc = "0~5V输入1",      pin = 77, gpio = 4,  scale = EXT_DIV_SCALE, offset = 200},
    {id = 3, name = "ADC3_5V_IN2",  desc = "0~5V输入2",      pin = 76, gpio = 28, scale = EXT_DIV_SCALE, offset = 200},
}

--[[
4-20mA电流换算系数(示例，根据实际采样电阻R18调整)：
  采样电阻上电压 Vr18 = Vin / VOLT_SCALE × (R18 + 分压等效阻抗) / ...
  简化：若R18很小(如250Ω)，且后端分压阻抗 >> R18，则近似：
  I(mA) = Vadc(mV) / R18(Ω) × (VOLT_SCALE)
  例 R18=412Ω: I = Vadc_mV / 412 × 3.8302
  如需启用电流换算，设置 ADC1_R18_OHM > 0
]]
local ADC1_R18_OHM = 412  -- 4-20mA采样电阻(欧姆)，设为0则不做电流换算


-- 初始化ADC分压电阻使能GPIO + 打开ADC通道
-- 重要: adc.setRange()必须在adc.open()之前设置,否则无效(官方demo说明)
-- 量程选择:
--   adc.ADC_RANGE_MIN = 0~1.5V (小电压更精准)
--   adc.ADC_RANGE_MAX = 0~3.3V
-- 本工程外接150k+53k分压,分压后电压在0~1.5V范围内,使用ADC_RANGE_MIN
local function init_channels()
    for _, ch in ipairs(ADC_CHANNELS) do
        -- 步骤1: GPIO拉低使能分压(下端接地)
        gpio.setup(ch.gpio, 0)
        -- 步骤2: 设置ADC量程(必须在open之前!)
        adc.setRange(adc.ADC_RANGE_MIN)
        -- 步骤3: 打开ADC通道
        local result = adc.open(ch.id)
        log.info("adc_test.open", ch.name, "id="..ch.id, "pin="..ch.pin, "range=MIN", result and "ok" or "fail")
    end
end

-- 关闭ADC通道 + 分压GPIO浮空(休眠时切断分压回路漏电流)
local function deinit_channels()
    for _, ch in ipairs(ADC_CHANNELS) do
        adc.close(ch.id)
        -- 分压使能GPIO设为输入浮空，断开分压回路，消除漏电流
        gpio.setup(ch.gpio, nil)
    end
    log.info("adc_test", "channels closed, div gpio float(省电模式)")
end

init_channels()


-- 挂起标志，休眠期间停止采样并关闭ADC(配合sleep_test模块的低功耗休眠)
local suspended = false

sys.subscribe("APP_SUSPEND", function()
    suspended = true
end)

sys.subscribe("APP_RESUME", function()
    suspended = false
end)


-- 单路ADC采样并换算
local function sample_one(ch)
    -- 使用adc.get(id)读取,返回值为毫伏数(对齐官方demo写法)
    local vadc_mv = adc.get(ch.id)
    if not vadc_mv or vadc_mv < 0 then
        log.error("adc_test.sample", ch.name, "read fail")
        return nil
    end

    -- vadc_mv 为 ADC引脚实际采样到的毫伏数(分压后)
    -- 换算成实际输入电压(毫伏)，使用每路独立的scale系数 + offset偏移校正
    local vin_mv = vadc_mv * ch.scale + ch.offset

    local log_str = string.format("channel=%s vadc=%.2fmV vin=%.2fmV (%.3fV) offset=%dmV",
        ch.name, vadc_mv, vin_mv, vin_mv / 1000, ch.offset)

    -- ADC1为4-20mA通道，若配置了采样电阻则额外换算电流
    if ch.id == 1 and ADC1_R18_OHM > 0 then
        -- 电流(mA) = 校正后的实际电压(mV) / 采样电阻(Ω)
        local current_ma = vin_mv / ADC1_R18_OHM
        log_str = string.format("%s I=%.3fmA", log_str, current_ma)
    end

    log.info("adc_test", log_str)
    return vin_mv
end


-- ADC采样task处理函数
local function adc_test_task_func()
    -- 等待1秒，硬件稳定
    sys.wait(1000)

    while true do
        if suspended then
            -- 已挂起: 关闭ADC省电，阻塞等待恢复消息
            deinit_channels()
            sys.waitUntil("APP_RESUME")
            -- 恢复: 重新初始化ADC通道
            init_channels()
        else
            log.info("adc_test", "---- sample start ----")

            for _, ch in ipairs(ADC_CHANNELS) do
                sample_one(ch)
            end

            log.info("adc_test", "---- sample end ----")
            -- 等待下一次采样
            sys.wait(SAMPLE_INTERVAL_MS)
        end
    end
end


log.info("adc_test", "init done, R_UP="..R_UP.."k R_DOWN="..R_DOWN.."k ext_div_scale=×"..string.format("%.4f", EXT_DIV_SCALE))

-- 创建并且启动一个task，运行ADC采样任务
sys.taskInit(adc_test_task_func)
