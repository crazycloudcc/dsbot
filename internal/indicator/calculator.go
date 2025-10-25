package indicator

import (
	"math"

	"dsbot/internal/models"
)

// Calculator 技术指标计算器
type Calculator struct {
	config *IndicatorConfig
}

// NewCalculator 创建计算器实例，使用默认配置
func NewCalculator() *Calculator {
	return &Calculator{
		config: DefaultConfig(),
	}
}

// NewCalculatorWithConfig 创建计算器实例，使用自定义配置
// 可以为不同的交易对使用不同的配置
func NewCalculatorWithConfig(config *IndicatorConfig) *Calculator {
	if config == nil {
		config = DefaultConfig()
	}
	return &Calculator{
		config: config,
	}
}

// Calculate 计算所有技术指标
func (c *Calculator) Calculate(ohlcvList []models.OHLCV) *models.TechnicalData {
	if len(ohlcvList) == 0 {
		return nil
	}

	closes := extractCloses(ohlcvList)
	highs := extractHighs(ohlcvList)
	lows := extractLows(ohlcvList)
	volumes := extractVolumes(ohlcvList)

	data := &models.TechnicalData{
		SMA5:     c.calculateSMA(closes, c.config.SMA5Period),
		SMA20:    c.calculateSMA(closes, c.config.SMA20Period),
		SMA50:    c.calculateSMA(closes, c.config.SMA50Period),
		RSI:      c.calculateRSI(closes, c.config.RSIPeriod),
		VolumeMA: c.calculateSMA(volumes, c.config.VolumeMAPeriod),
	}

	// EMA和MACD
	data.EMA12 = c.calculateEMA(closes, c.config.EMA12Period)
	data.EMA26 = c.calculateEMA(closes, c.config.EMA26Period)
	data.MACD = data.EMA12 - data.EMA26

	// 计算MACD信号线
	macdValues := c.calculateMACDValues(closes, c.config.MACDFastPeriod, c.config.MACDSlowPeriod)
	data.MACDSignal = c.calculateEMAFromValues(macdValues, c.config.MACDSignalPeriod)
	data.MACDHistogram = data.MACD - data.MACDSignal

	// 布林带
	bbMiddle, bbUpper, bbLower := c.calculateBollingerBands(closes, c.config.BBPeriod, c.config.BBStdDev)
	data.BBMiddle = bbMiddle
	data.BBUpper = bbUpper
	data.BBLower = bbLower
	// 计算价格在布林带中的位置，避免除以零
	if bbUpper > bbLower && (bbUpper-bbLower) > c.config.BBMinThreshold {
		currentPrice := closes[len(closes)-1]
		data.BBPosition = (currentPrice - bbLower) / (bbUpper - bbLower)
	} else {
		data.BBPosition = c.config.BBDefaultPos // 中间位置
	}

	// 成交量比率 - 当前成交量与平均成交量的比值
	if len(volumes) > 0 && data.VolumeMA > 0 {
		currentVolume := volumes[len(volumes)-1]
		data.VolumeRatio = currentVolume / data.VolumeMA
	} else {
		data.VolumeRatio = c.config.DefaultVolumeRatio
	}

	// 支撑阻力位 - 使用最近N个周期的最高最低点
	lookbackPeriod := c.config.SupportResistanceLookback
	if len(highs) < lookbackPeriod {
		lookbackPeriod = len(highs)
	}
	if lookbackPeriod > 0 {
		startIdx := len(highs) - lookbackPeriod
		data.Resistance = max(highs[startIdx:])
		data.Support = min(lows[startIdx:])
	} else {
		// 数据不足时使用当前价格
		currentPrice := closes[len(closes)-1]
		data.Resistance = currentPrice
		data.Support = currentPrice
	}

	return data
}

// CalculateTrendAnalysis 计算趋势分析
func (c *Calculator) CalculateTrendAnalysis(ohlcvList []models.OHLCV, tech *models.TechnicalData) *models.TrendAnalysis {
	if len(ohlcvList) == 0 || tech == nil {
		return nil
	}

	currentPrice := ohlcvList[len(ohlcvList)-1].Close

	shortTerm := "下跌"
	if currentPrice > tech.SMA20 {
		shortTerm = "上涨"
	}

	mediumTerm := "下跌"
	if currentPrice > tech.SMA50 {
		mediumTerm = "上涨"
	}

	macdTrend := "bearish"
	if tech.MACD > tech.MACDSignal {
		macdTrend = "bullish"
	}

	overall := "震荡整理"
	if shortTerm == "上涨" && mediumTerm == "上涨" {
		overall = "强势上涨"
	} else if shortTerm == "下跌" && mediumTerm == "下跌" {
		overall = "强势下跌"
	}

	return &models.TrendAnalysis{
		ShortTerm:  shortTerm,
		MediumTerm: mediumTerm,
		MACD:       macdTrend,
		Overall:    overall,
		RSILevel:   tech.RSI,
	}
}

// CalculateLevelsAnalysis 计算支撑阻力位
func (c *Calculator) CalculateLevelsAnalysis(ohlcvList []models.OHLCV, tech *models.TechnicalData) *models.LevelsAnalysis {
	if len(ohlcvList) == 0 || tech == nil {
		return nil
	}

	currentPrice := ohlcvList[len(ohlcvList)-1].Close
	lookback := c.config.SupportResistanceLookback
	if len(ohlcvList) < lookback {
		lookback = len(ohlcvList)
	}

	if lookback == 0 {
		return &models.LevelsAnalysis{
			StaticResistance:  currentPrice,
			StaticSupport:     currentPrice,
			DynamicResistance: tech.BBUpper,
			DynamicSupport:    tech.BBLower,
			PriceVsResistance: 0,
			PriceVsSupport:    0,
		}
	}

	recentData := ohlcvList[len(ohlcvList)-lookback:]
	highs := extractHighs(recentData)
	lows := extractLows(recentData)

	staticResistance := max(highs)
	staticSupport := min(lows)

	// 计算价格与支撑阻力位的距离百分比
	// 阻力位：(阻力 - 当前价) / 当前价 * 100 = 还有多少上涨空间
	// 支撑位：(当前价 - 支撑) / 支撑 * 100 = 已经上涨了多少
	priceVsResistance := 0.0
	priceVsSupport := 0.0

	if currentPrice > 0 {
		priceVsResistance = ((staticResistance - currentPrice) / currentPrice) * 100
	}
	if staticSupport > 0 {
		priceVsSupport = ((currentPrice - staticSupport) / staticSupport) * 100
	}

	return &models.LevelsAnalysis{
		StaticResistance:  staticResistance,
		StaticSupport:     staticSupport,
		DynamicResistance: tech.BBUpper,
		DynamicSupport:    tech.BBLower,
		PriceVsResistance: priceVsResistance,
		PriceVsSupport:    priceVsSupport,
	}
}

// SMA 简单移动平均线
func (c *Calculator) calculateSMA(values []float64, period int) float64 {
	if len(values) == 0 {
		return 0
	}

	// 如果数据不足，使用所有可用数据
	if len(values) < period {
		period = len(values)
	}

	if period == 0 {
		return 0
	}

	// 计算最近 period 个值的平均
	sum := 0.0
	startIdx := len(values) - period
	for i := startIdx; i < len(values); i++ {
		sum += values[i]
	}
	return sum / float64(period)
}

// EMA 指数移动平均线
func (c *Calculator) calculateEMA(values []float64, period int) float64 {
	if len(values) == 0 {
		return 0
	}

	// 数据不足时使用所有数据计算 SMA
	if len(values) < period {
		return c.calculateSMA(values, len(values))
	}

	// EMA 的平滑因子
	multiplier := 2.0 / float64(period+1)

	// 初始 EMA 使用前 period 个数据的 SMA
	ema := c.calculateSMA(values[:period], period)

	// 从 period 开始迭代计算 EMA
	for i := period; i < len(values); i++ {
		ema = (values[i] * multiplier) + (ema * (1 - multiplier))
	}

	return ema
}

// EMAFromValues 从已有值计算EMA
func (c *Calculator) calculateEMAFromValues(values []float64, period int) float64 {
	return c.calculateEMA(values, period)
}

// calculateMACDValues 计算MACD值序列 - 优化版本
func (c *Calculator) calculateMACDValues(closes []float64, fast, slow int) []float64 {
	if len(closes) < slow {
		return []float64{0}
	}

	// 计算整个周期的 EMA12 和 EMA26
	// 为了获得信号线，我们需要 MACD 的历史值
	minLength := slow
	if len(closes) < minLength {
		return []float64{0}
	}

	macdValues := make([]float64, 0)

	// 从有足够数据开始计算
	for i := slow; i <= len(closes); i++ {
		subset := closes[:i]
		ema12 := c.calculateEMA(subset, fast)
		ema26 := c.calculateEMA(subset, slow)
		macdValues = append(macdValues, ema12-ema26)
	}

	// 如果数据不足，至少返回当前 MACD 值
	if len(macdValues) == 0 {
		ema12 := c.calculateEMA(closes, fast)
		ema26 := c.calculateEMA(closes, slow)
		macdValues = append(macdValues, ema12-ema26)
	}

	return macdValues
}

// RSI 相对强弱指数 - 使用 Wilder's Smoothing Method
func (c *Calculator) calculateRSI(values []float64, period int) float64 {
	if len(values) < period+1 {
		return c.config.RSINeutralValue // 数据不足时返回中性值
	}

	// 计算价格变化
	changes := make([]float64, len(values)-1)
	for i := 1; i < len(values); i++ {
		changes[i-1] = values[i] - values[i-1]
	}

	if len(changes) < period {
		return c.config.RSINeutralValue
	}

	// 分离涨跌
	gains := make([]float64, len(changes))
	losses := make([]float64, len(changes))
	for i, change := range changes {
		if change > 0 {
			gains[i] = change
			losses[i] = 0
		} else {
			gains[i] = 0
			losses[i] = -change
		}
	}

	// 使用 Wilder's Smoothing (类似于 EMA)
	// 第一个周期使用 SMA
	avgGain := 0.0
	avgLoss := 0.0
	for i := 0; i < period; i++ {
		avgGain += gains[i]
		avgLoss += losses[i]
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	// 后续周期使用平滑方法
	for i := period; i < len(gains); i++ {
		avgGain = (avgGain*float64(period-1) + gains[i]) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + losses[i]) / float64(period)
	}

	// 避免除以零
	if avgLoss == 0 {
		if avgGain == 0 {
			return c.config.RSINeutralValue
		}
		return c.config.RSIMaxValue
	}

	rs := avgGain / avgLoss
	rsi := c.config.RSIMaxValue - (c.config.RSIMaxValue / (1.0 + rs))

	return rsi
}

// BollingerBands 布林带
func (c *Calculator) calculateBollingerBands(values []float64, period int, stdDev float64) (middle, upper, lower float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}

	if len(values) < period {
		period = len(values)
	}

	if period == 0 {
		return 0, 0, 0
	}

	// 计算中轨（SMA）
	middle = c.calculateSMA(values, period)

	// 计算标准差
	variance := 0.0
	startIdx := len(values) - period
	for i := startIdx; i < len(values); i++ {
		diff := values[i] - middle
		variance += diff * diff
	}
	std := math.Sqrt(variance / float64(period))

	// 计算上下轨
	upper = middle + (std * stdDev)
	lower = middle - (std * stdDev)

	// 确保上下轨不相等（避免除以零）
	if upper == lower {
		upper = middle * c.config.BBUpperAdjust
		lower = middle * c.config.BBLowerAdjust
	}

	return
}

// 辅助函数

func extractCloses(ohlcvList []models.OHLCV) []float64 {
	closes := make([]float64, len(ohlcvList))
	for i, ohlcv := range ohlcvList {
		closes[i] = ohlcv.Close
	}
	return closes
}

func extractHighs(ohlcvList []models.OHLCV) []float64 {
	highs := make([]float64, len(ohlcvList))
	for i, ohlcv := range ohlcvList {
		highs[i] = ohlcv.High
	}
	return highs
}

func extractLows(ohlcvList []models.OHLCV) []float64 {
	lows := make([]float64, len(ohlcvList))
	for i, ohlcv := range ohlcvList {
		lows[i] = ohlcv.Low
	}
	return lows
}

func extractVolumes(ohlcvList []models.OHLCV) []float64 {
	volumes := make([]float64, len(ohlcvList))
	for i, ohlcv := range ohlcvList {
		volumes[i] = ohlcv.Volume
	}
	return volumes
}

func max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxVal := values[0]
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

func min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minVal := values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
	}
	return minVal
}
