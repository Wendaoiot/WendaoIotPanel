--[[
@module  adc_4_20ma
@summary 4-20mA 电流环采集（ADC1 / PIN96），换算 mA 与百分比并上报平台
@version 1.0

@usage
采集链路（38盒子 V3.1 原理图）：
  4~20mA_INPUT ──► 采样电阻 R18(250Ω，0603) 到地，电流变为电压
                    4mA→1V，20mA→5V
                 ──► R20(150k)/R21(53k) 分压（R21 下端由 GPIO20 拉低接地使能）
                    分压比 53/(150+53)=0.261，5V→1.305V，在 ADC 1.5V 量程内
                 ──► ADC1（模组 PIN96）

换算：
  adc.get() 返回分压后的毫伏（固件已校准）
  vin_mv = vadc_mv × 3.8302 + 200        -- 3.8302=203/53 分压还原，200mV 为板级经验校正
  current_ma = vin_mv / R18(250Ω)        -- mV/Ω 数值上即 mA
  percent = (current_ma - 4) / 16 × 100  -- 4mA=0%，20mA=100%

上报（每 2 秒，wendao/<IMEI>/data）：
  {"id":"msg_...","ts":...,"tags":{"current_ma":12.01,"percent":50.1}}

现场标定建议：输入 4mA 和 20mA，对照上报值微调 OFFSET_MV 与 R18_OHM。
]]

local mqtt_wendao = require "mqtt_wendao"

-- ===================== 配置区 =====================
local ADC_ID = 1             -- ADC 通道号（ADC1）
local DIV_GPIO = 20          -- 分压使能脚 GPIO20：输出低电平=R21 下端接地=使能采样
local SAMPLE_PERIOD_MS = 2000 -- 采样/上报周期

local R18_OHM = 250          -- 4-20mA 采样电阻阻值（38盒子为 250Ω；按实际板调整）
local EXT_DIV_SCALE = 203 / 53  -- 分压还原系数 (150+53)/53 ≈ 3.8302
local OFFSET_MV = 200        -- 板级电压校正偏移(mV)：读数偏小调大、偏强调小
-- 百分比量程（标准 4-20mA = 4mA 零点 / 20mA 满程）
local MA_MIN, MA_MAX = 4, 20
-- ===================================================
-- ADC 量程 0~1.5V（分流后最高 1.305V，用小量程更精准）
-- 注意：setRange 必须在 adc.open 之前调用

-- 初始化：先拉低分压使能脚（导通分压回路），再设置量程并打开 ADC
gpio.setup(DIV_GPIO, 0)
adc.setRange(adc.ADC_RANGE_MIN)
adc.open(ADC_ID)

log.info("adc_4_20ma", "init done: ADC" .. ADC_ID .. " div_gpio=GPIO" .. DIV_GPIO,
    "R18=" .. R18_OHM .. "ohm scale=" .. string.format("%.4f", EXT_DIV_SCALE) .. " off=" .. OFFSET_MV .. "mV")

-- 采集任务：周期采样 → 换算 → 上报
sys.taskInit(function()
    sys.wait(1000)   -- 等硬件稳定
    while true do
        local vadc_mv = adc.get(ADC_ID)   -- 分压后的毫伏值
        if vadc_mv and vadc_mv >= 0 then
            -- 还原采样电阻上的电压 → 电流 → 量程百分比
            local vin_mv = vadc_mv * EXT_DIV_SCALE + OFFSET_MV
            local current_ma = vin_mv / R18_OHM
            local percent = (current_ma - MA_MIN) / (MA_MAX - MA_MIN) * 100

            -- 断线检测：电流 < 3.5mA 认为回路断开（标准 4-20mA 断线时电流为 0）
            if current_ma < MA_MIN - 0.5 then
                log.warn("adc_4_20ma", "loop broken? current=", string.format("%.2f", current_ma) .. "mA")
                mqtt_wendao.report({ current_ma = current_ma, percent = -1, fault = "loop_broken" })
            else
                log.info("adc_4_20ma", string.format("I=%.2fmA (%.1f%%)", current_ma, percent))
                -- 百分比钳制在 0~100
                if percent < 0 then percent = 0 end
                if percent > 100 then percent = 100 end
                mqtt_wendao.report({ current_ma = current_ma, percent = percent })
            end
        else
            log.error("adc_4_20ma", "adc.get fail")
        end
        sys.wait(SAMPLE_PERIOD_MS)
    end
end)
