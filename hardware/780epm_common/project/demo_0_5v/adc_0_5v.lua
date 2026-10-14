--[[
@module  adc_0_5v
@summary 0-5V 电压采集（ADC2 / PIN77，端子 0~5V_INPUT1），换算电压并上报平台
@version 1.0

@usage
采集链路（38盒子 V3.1 原理图）：
  0~5V_INPUT1 ──► R39(150k)/R40(53k) 分压（R40 下端由 GPIO4 拉低接地使能）
                   分压比 53/(150+53)=0.261，5V→1.305V，在 ADC 1.5V 量程内
                 ──► 电容 C24 滤波 ──► ADC2（模组 PIN77）
  板载另一路 0~5V：0~5V_INPUT2 → ADC3（PIN76），使能 GPIO28（见文件末尾改法）

换算：
  adc.get() 返回分压后的毫伏（固件已校准）
  vin_mv = vadc_mv × 3.8302 + 200    -- 3.8302=203/53 分压还原，200mV 为板级经验校正
  voltage_v = vin_mv / 1000
  分压网络最大可测 ≈ 1.6V × 3.8302 ≈ 6.1V（请勿长期超过 5V 额定输入）

上报（每 2 秒，wendao/<IMEI>/data）：
  {"id":"msg_...","ts":...,"tags":{"voltage_v":2.497}}

现场标定建议：输入 0V 和 5.000V，对照上报值微调 OFFSET_MV（必要时微调 EXT_DIV_SCALE）。
]]

local mqtt_wendao = require "mqtt_wendao"

-- ===================== 配置区 =====================
local ADC_ID = 2             -- ADC 通道号（ADC2 = 0~5V_INPUT1）
local DIV_GPIO = 4           -- 分压使能脚 GPIO4：输出低电平=分压电阻下端接地=使能采样
local SAMPLE_PERIOD_MS = 2000 -- 采样/上报周期

local EXT_DIV_SCALE = 203 / 53  -- 分压还原系数 (150+53)/53 ≈ 3.8302
local OFFSET_MV = 200        -- 板级电压校正偏移(mV)：读数偏小调大、偏强调小
-- ===================================================
-- ADC 量程 0~1.5V（分流后最高 1.305V，用小量程更精准）
-- 注意：setRange 必须在 adc.open 之前调用

-- 初始化：先拉低分压使能脚（导通分压回路），再设置量程并打开 ADC
gpio.setup(DIV_GPIO, 0)
adc.setRange(adc.ADC_RANGE_MIN)
adc.open(ADC_ID)

log.info("adc_0_5v", "init done: ADC" .. ADC_ID .. " div_gpio=GPIO" .. DIV_GPIO,
    "scale=" .. string.format("%.4f", EXT_DIV_SCALE) .. " off=" .. OFFSET_MV .. "mV")

-- 采集任务：周期采样 → 换算 → 上报
sys.taskInit(function()
    sys.wait(1000)   -- 等硬件稳定
    while true do
        local vadc_mv = adc.get(ADC_ID)   -- 分压后的毫伏值
        if vadc_mv and vadc_mv >= 0 then
            -- 分压还原 + 板级校正，得到实际输入电压
            local vin_mv = vadc_mv * EXT_DIV_SCALE + OFFSET_MV
            local voltage_v = vin_mv / 1000

            -- 轻微钳制（不允许负电压读数显示出来）
            if voltage_v < 0 then voltage_v = 0 end

            log.info("adc_0_5v", string.format("V=%.3fV (vadc=%dmV)", voltage_v, vadc_mv))
            mqtt_wendao.report({ voltage_v = voltage_v })
        else
            log.error("adc_0_5v", "adc.get fail")
        end
        sys.wait(SAMPLE_PERIOD_MS)
    end
end)

--[[
使用另一路 0-5V（端子 0~5V_INPUT2）只需改上面配置区两行：
    local ADC_ID = 3      -- ADC3
    local DIV_GPIO = 28  -- 使能脚 GPIO28（对应模组 PIN76）
]]
