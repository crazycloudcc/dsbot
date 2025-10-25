package models

import (
	"fmt"
	"time"
)

// OHLCV K线数据
type OHLCV struct {
	Timestamp time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
}

// Ticker 行情数据
type Ticker struct {
	Symbol string
	Last   float64 // 最新成交价
	Bid    float64 // 买一价
	Ask    float64 // 卖一价
}

// TechnicalData 技术指标数据
type TechnicalData struct {
	SMA5          float64
	SMA20         float64
	SMA50         float64
	EMA12         float64
	EMA26         float64
	MACD          float64
	MACDSignal    float64
	MACDHistogram float64
	RSI           float64
	BBUpper       float64
	BBMiddle      float64
	BBLower       float64
	BBPosition    float64
	VolumeMA      float64
	VolumeRatio   float64
	Resistance    float64
	Support       float64
}

// TrendAnalysis 趋势分析
type TrendAnalysis struct {
	ShortTerm  string
	MediumTerm string
	MACD       string
	Overall    string
	RSILevel   float64
}

// LevelsAnalysis 支撑阻力分析
type LevelsAnalysis struct {
	StaticResistance  float64
	StaticSupport     float64
	DynamicResistance float64
	DynamicSupport    float64
	PriceVsResistance float64
	PriceVsSupport    float64
}

// MarketData 市场数据
type MarketData struct {
	Price          float64
	Timestamp      string
	High           float64
	Low            float64
	Volume         float64
	Timeframe      string
	PriceChange    float64
	KlineData      []OHLCV
	TechnicalData  *TechnicalData
	TrendAnalysis  *TrendAnalysis
	LevelsAnalysis *LevelsAnalysis
}

// Position 持仓信息
type Position struct {
	Side          string // "long" or "short"
	Size          float64
	EntryPrice    float64
	UnrealizedPnL float64
	Leverage      int
	Symbol        string
	StopLoss      float64 // 止损价格
	TakeProfit    float64 // 止盈价格
	TrailingStop  float64 // 移动止损价格（动态更新）
	HighestPrice  float64 // 开仓后的最高价（用于移动止损）
	LowestPrice   float64 // 开仓后的最低价（用于移动止损）
}

// TradeSignal 交易信号
type TradeSignal struct {
	Signal      string `json:"signal"`       // "BUY", "SELL", "HOLD"
	Reason      string `json:"reason"`       // 交易理由
	Confidence  string `json:"confidence"`   // "HIGH", "MEDIUM", "LOW"
	Timestamp   string `json:"timestamp"`    // 时间戳
	IsFallback  bool   `json:"is_fallback"`  // 是否为备用信号
	TradingPair string `json:"trading_pair"` // 交易对标识 (如 "BTC-USDT")
}

// SignalStats 信号统计
type SignalStats struct {
	BuyCount  int // BUY信号次数
	SellCount int // SELL信号次数
	HoldCount int // HOLD信号次数
	Total     int // 总信号次数
}

// FormatStats 格式化统计信息
func (s *SignalStats) FormatStats() string {
	if s.Total == 0 {
		return "(BUY=0/0, SELL=0/0, HOLD=0/0)"
	}
	return fmt.Sprintf("(BUY=%d/%d, SELL=%d/%d, HOLD=%d/%d)",
		s.BuyCount, s.Total,
		s.SellCount, s.Total,
		s.HoldCount, s.Total)
}

// SessionContext AI会话上下文 (用于隔离不同交易对的对话历史)
type SessionContext struct {
	TradingPair   string        // 交易对标识
	SignalHistory []TradeSignal // 该交易对的信号历史
	LastUpdate    string        // 最后更新时间
	Stats         SignalStats   // 信号统计
}
