package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type ExchangeType string

const (
	ExchangeOKX     ExchangeType = "okx" // default
	ExchangeBinance ExchangeType = "binance"
)

// TradingMode 交易模式
type TradingMode string

const (
	TradingModeSpot    TradingMode = "spot"    // 现货交易
	TradingModeFutures TradingMode = "futures" // 合约交易 (default)
)

// Config 全局配置结构
type Config struct {
	Trading TradingConfig `json:"trading"`
	API     APIConfig     `json:"api"`
	Logging LoggingConfig `json:"logging"`
}

// TradingConfig 交易配置
type TradingConfig struct {
	SymbolA                 string               `json:"symbolA"`
	SymbolB                 string               `json:"symbolB"`
	Amount                  float64              `json:"amount"` // 交易金额，单位为symbolB（如USDT、USDT）
	Leverage                int                  `json:"leverage"`
	Timeframe               string               `json:"timeframe"`
	TestMode                bool                 `json:"test_mode"`
	DataPoints              int                  `json:"data_points"`
	ScheduleIntervalMinutes int                  `json:"schedule_interval_minutes"`
	TradingMode             string               `json:"trading_mode"`    // "spot" or "futures" (default: futures)
	RiskManagement          RiskManagementConfig `json:"risk_management"` // 风险管理配置
}

// RiskManagementConfig 风险管理配置
type RiskManagementConfig struct {
	EnableStopLoss       bool    `json:"enable_stop_loss"`       // 是否启用止损
	EnableTakeProfit     bool    `json:"enable_take_profit"`     // 是否启用止盈
	StopLossPercent      float64 `json:"stop_loss_percent"`      // 止损百分比（如2.0表示2%）
	TakeProfitPercent    float64 `json:"take_profit_percent"`    // 止盈百分比（如4.0表示4%）
	EnableTrailingStop   bool    `json:"enable_trailing_stop"`   // 是否启用移动止损
	TrailingStopDistance float64 `json:"trailing_stop_distance"` // 移动止损距离（%）
	CheckIntervalSeconds int     `json:"check_interval_seconds"` // 检查间隔（秒）
}

// APIConfig API配置
type APIConfig struct {
	DeepSeekAPIKey  string `json:"deepseek_api_key"`
	DeepSeekBaseURL string `json:"deepseek_base_url"`
	OKXAPIKey       string `json:"okx_api_key"`
	OKXSecret       string `json:"okx_secret"`
	OKXPassword     string `json:"okx_password"`
	BinanceAPIKey   string `json:"binance_api_key"`
	BinanceSecret   string `json:"binance_secret"`
	ExchangeType    string `json:"exchange_type"` // "okx" or "binance"
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	LogLevelConsole   string `json:"log_level_console"`
	LogLevelFile      string `json:"log_level_file"`
	LogDir            string `json:"log_dir"`
	EnableFileLogging bool   `json:"enable_file_logging"`
}

// LoadConfig 从JSON文件和环境变量加载配置
func LoadConfig(configPath string) (*Config, error) {
	// 读取配置文件
	file, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(file, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 从环境变量覆盖敏感信息
	if apiKey := os.Getenv("DEEPSEEK_API_KEY"); apiKey != "" {
		cfg.API.DeepSeekAPIKey = apiKey
	}
	if apiKey := os.Getenv("OKX_API_KEY"); apiKey != "" {
		cfg.API.OKXAPIKey = apiKey
	}
	if secret := os.Getenv("OKX_SECRET"); secret != "" {
		cfg.API.OKXSecret = secret
	}
	if password := os.Getenv("OKX_PASSWORD"); password != "" {
		cfg.API.OKXPassword = password
	}
	if apiKey := os.Getenv("BINANCE_API_KEY"); apiKey != "" {
		cfg.API.BinanceAPIKey = apiKey
	}
	if secret := os.Getenv("BINANCE_SECRET"); secret != "" {
		cfg.API.BinanceSecret = secret
	}

	// 验证必需配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate 验证配置有效性
func (c *Config) Validate() error {
	if c.API.DeepSeekAPIKey == "" {
		return fmt.Errorf("DeepSeek API Key 未配置")
	}

	// 验证交易所配置
	exchangeType := c.API.ExchangeType

	switch exchangeType {
	case string(ExchangeOKX):
		if c.API.OKXAPIKey == "" || c.API.OKXSecret == "" || c.API.OKXPassword == "" {
			return fmt.Errorf("OKX API 凭证未完整配置")
		}
	case string(ExchangeBinance):
		if c.API.BinanceAPIKey == "" || c.API.BinanceSecret == "" {
			return fmt.Errorf("Binance API 凭证未完整配置")
		}
	default:
		return fmt.Errorf("不支持的交易所类型: %s (支持: okx, binance)", exchangeType)
	}

	if c.Trading.Amount <= 0 {
		return fmt.Errorf("交易数量必须大于0")
	}

	// 验证交易模式和杠杆配置
	tradingMode := c.Trading.TradingMode
	if tradingMode == "" {
		tradingMode = string(TradingModeFutures) // 默认合约模式
	}

	switch TradingMode(tradingMode) {
	case TradingModeSpot:
		// 现货交易不使用杠杆
		if c.Trading.Leverage != 1 && c.Trading.Leverage != 0 {
			fmt.Printf("警告: 现货交易模式不使用杠杆，杠杆配置 %dx 将被忽略\n", c.Trading.Leverage)
		}
	case TradingModeFutures:
		// 合约交易需要杠杆
		if c.Trading.Leverage <= 0 {
			return fmt.Errorf("合约交易模式下杠杆倍数必须大于0")
		}
	default:
		return fmt.Errorf("不支持的交易模式: %s (支持: spot, futures)", tradingMode)
	}

	return nil
}

// GetTradingMode 获取交易模式 (带默认值)
func (c *Config) GetTradingMode() TradingMode {
	if c.Trading.TradingMode == "" {
		return TradingModeFutures // 默认合约模式
	}
	return TradingMode(c.Trading.TradingMode)
}

// IsSpotMode 是否为现货模式
func (c *Config) IsSpotMode() bool {
	return c.GetTradingMode() == TradingModeSpot
}

// IsFuturesMode 是否为合约模式
func (c *Config) IsFuturesMode() bool {
	return c.GetTradingMode() == TradingModeFutures
}
