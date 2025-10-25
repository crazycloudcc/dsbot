package indicator

// IndicatorConfig 技术指标参数配置
// 可以为不同的交易对创建不同的配置实例
type IndicatorConfig struct {
	// 移动平均线周期
	SMA5Period  int
	SMA20Period int
	SMA50Period int

	// EMA 周期
	EMA12Period int
	EMA26Period int

	// MACD 参数
	MACDFastPeriod   int
	MACDSlowPeriod   int
	MACDSignalPeriod int

	// RSI 参数
	RSIPeriod       int
	RSINeutralValue float64 // RSI 中性值（数据不足时返回）
	RSIMaxValue     float64 // RSI 最大值

	// 布林带参数
	BBPeriod       int
	BBStdDev       float64
	BBMinThreshold float64 // 布林带最小阈值，避免除以零
	BBDefaultPos   float64 // 布林带默认位置（中间）
	BBUpperAdjust  float64 // 上轨调整系数
	BBLowerAdjust  float64 // 下轨调整系数

	// 成交量参数
	VolumeMAPeriod     int
	DefaultVolumeRatio float64 // 默认成交量比率

	// 支撑阻力参数
	SupportResistanceLookback int // 支撑阻力位回溯周期
}

// DefaultConfig 返回默认的技术指标配置
// 使用行业标准参数，适用于大多数交易场景
func DefaultConfig() *IndicatorConfig {
	return &IndicatorConfig{
		// 移动平均线周期
		SMA5Period:  5,
		SMA20Period: 20,
		SMA50Period: 50,

		// EMA 周期
		EMA12Period: 12,
		EMA26Period: 26,

		// MACD 参数
		MACDFastPeriod:   12,
		MACDSlowPeriod:   26,
		MACDSignalPeriod: 9,

		// RSI 参数
		RSIPeriod:       14,
		RSINeutralValue: 50.0,
		RSIMaxValue:     100.0,

		// 布林带参数
		BBPeriod:       20,
		BBStdDev:       2.0,
		BBMinThreshold: 0.0001,
		BBDefaultPos:   0.5,
		BBUpperAdjust:  1.001,
		BBLowerAdjust:  0.999,

		// 成交量参数
		VolumeMAPeriod:     20,
		DefaultVolumeRatio: 1.0,

		// 支撑阻力参数
		SupportResistanceLookback: 20,
	}
}

// AggressiveConfig 返回激进型配置
// 适合短线交易，使用较短的周期，对市场变化更敏感
func AggressiveConfig() *IndicatorConfig {
	return &IndicatorConfig{
		// 更短的移动平均线周期
		SMA5Period:  3,
		SMA20Period: 10,
		SMA50Period: 30,

		// 标准 EMA 周期
		EMA12Period: 12,
		EMA26Period: 26,

		// 标准 MACD 参数
		MACDFastPeriod:   12,
		MACDSlowPeriod:   26,
		MACDSignalPeriod: 9,

		// 更短的 RSI 周期
		RSIPeriod:       10,
		RSINeutralValue: 50.0,
		RSIMaxValue:     100.0,

		// 更短的布林带周期
		BBPeriod:       15,
		BBStdDev:       2.0,
		BBMinThreshold: 0.0001,
		BBDefaultPos:   0.5,
		BBUpperAdjust:  1.001,
		BBLowerAdjust:  0.999,

		// 更短的成交量周期
		VolumeMAPeriod:     15,
		DefaultVolumeRatio: 1.0,

		// 更短的支撑阻力回溯
		SupportResistanceLookback: 15,
	}
}

// ConservativeConfig 返回保守型配置
// 适合中长线交易，使用较长的周期，过滤短期噪音
func ConservativeConfig() *IndicatorConfig {
	return &IndicatorConfig{
		// 更长的移动平均线周期
		SMA5Period:  10,
		SMA20Period: 30,
		SMA50Period: 100,

		// 标准 EMA 周期
		EMA12Period: 12,
		EMA26Period: 26,

		// 标准 MACD 参数
		MACDFastPeriod:   12,
		MACDSlowPeriod:   26,
		MACDSignalPeriod: 9,

		// 更长的 RSI 周期
		RSIPeriod:       21,
		RSINeutralValue: 50.0,
		RSIMaxValue:     100.0,

		// 更长的布林带周期
		BBPeriod:       30,
		BBStdDev:       2.5,
		BBMinThreshold: 0.0001,
		BBDefaultPos:   0.5,
		BBUpperAdjust:  1.001,
		BBLowerAdjust:  0.999,

		// 更长的成交量周期
		VolumeMAPeriod:     30,
		DefaultVolumeRatio: 1.0,

		// 更长的支撑阻力回溯
		SupportResistanceLookback: 30,
	}
}
