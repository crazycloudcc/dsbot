package exchange

import (
	"dsbot/internal/models"
)

// Exchange 交易所接口 - 使用依赖注入模式，支持多交易所扩展
type Exchange interface {
	// FetchOHLCV 获取K线数据
	// symbol: 交易对符号 (如 "BTC/USDT:USDT", "BTC/USDT")
	// timeframe: 时间周期 (如 "5m", "15m", "1h")
	// limit: 数据条数
	FetchOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error)

	// FetchTicker 获取最新行情
	// symbol: 交易对符号
	FetchTicker(symbol string) (*models.Ticker, error)

	// FetchPosition 获取持仓信息（合约模式）
	// symbol: 交易对符号
	FetchPosition(symbol string) (*models.Position, error)

	// FetchBalance 获取账户余额（现货模式）
	// currency: 币种 (如 "BTC", "USDT")
	FetchBalance(currency string) (float64, error)

	// PlaceOrder 下单
	// symbol: 交易对符号
	// side: 买卖方向 ("buy" or "sell")
	// amount: 数量
	// params: 额外参数 (如 reduceOnly, posSide 等)
	PlaceOrder(symbol, side string, amount float64, params map[string]interface{}) error

	// SetLeverage 设置杠杆
	// symbol: 交易对符号
	// leverage: 杠杆倍数
	SetLeverage(symbol string, leverage int) error

	// GetInstrumentInfo 获取交易对信息
	// symbol: 交易对符号
	GetInstrumentInfo(symbol string) (*InstrumentInfo, error)

	// ParseSymbols 解析交易对符号
	// symbolA: 基础币种 (如 "BTC")
	// symbolB: 计价币种 (如 "USDT")
	// 返回: 交易所特定的符号格式
	ParseSymbols(symbolA, symbolB string) string

	// GetExchangeName 获取交易所名称
	GetExchangeName() string
}

// InstrumentInfo 合约信息 (通用结构)
type InstrumentInfo struct {
	InstID        string  // 合约ID
	ContractValue float64 // 合约面值
	LotSize       float64 // 下单数量精度
	MinSize       float64 // 最小下单数量
	MinAmount     float64 // 最小订单金额（现货专用，以计价货币计）
	TickSize      float64 // 价格精度
}
