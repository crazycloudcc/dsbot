package exchange

import (
	"fmt"

	"dsbot/internal/config"
)

// NewExchange 交易所工厂函数 - 根据配置创建对应的交易所客户端
func NewExchange(cfg *config.APIConfig, tradingMode config.TradingMode) (Exchange, error) {
	exchangeType := cfg.ExchangeType

	switch exchangeType {
	case string(config.ExchangeOKX):
		client := NewOKXClient(cfg, tradingMode)
		if client == nil {
			return nil, fmt.Errorf("创建OKX客户端失败")
		}
		return client, nil

	default:
		return nil, fmt.Errorf("不支持的交易所类型: %s (支持: okx, binance)", exchangeType)
	}
}

// GetSupportedExchanges 获取支持的交易所列表
func GetSupportedExchanges() []string {
	return []string{"okx", "binance"}
}
