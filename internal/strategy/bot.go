package strategy

import (
	"fmt"
	"time"

	"dsbot/internal/ai"
	"dsbot/internal/config"
	"dsbot/internal/exchange"
	"dsbot/internal/indicator"
	"dsbot/internal/logger"
	"dsbot/internal/models"
)

// TradingBot 交易机器人
type TradingBot struct {
	config          *config.Config
	exchange        exchange.Exchange // 使用接口而不是具体实现
	aiClient        *ai.DeepSeekClient
	calculator      *indicator.Calculator
	currentPosition *models.Position
	tradingPair     string       // 交易对标识 (如 "BTC-USDT")
	riskManager     *RiskManager // 风险管理器
}

// NewTradingBot 创建交易机器人 - 使用依赖注入
func NewTradingBot(cfg *config.Config, exch exchange.Exchange, aiClient *ai.DeepSeekClient) *TradingBot {
	// 构建交易对标识
	tradingPair := fmt.Sprintf("%s-%s", cfg.Trading.SymbolA, cfg.Trading.SymbolB)

	bot := &TradingBot{
		config:      cfg,
		exchange:    exch,
		aiClient:    aiClient,
		calculator:  indicator.NewCalculatorWithConfig(indicator.AggressiveConfig()), // indicator.NewCalculator(),
		tradingPair: tradingPair,
	}

	// 创建风险管理器（仅在合约模式下）
	if cfg.IsFuturesMode() &&
		(cfg.Trading.RiskManagement.EnableStopLoss || cfg.Trading.RiskManagement.EnableTakeProfit) {
		bot.riskManager = NewRiskManager(cfg, exch, tradingPair)
	}

	return bot
}

// Run 执行交易流程
func (bot *TradingBot) Run() error {
	logger.Println("============================================================")
	logger.Printf("执行时间: %s", time.Now().Format("2006-01-02 15:04:05"))
	logger.Println("============================================================")

	// 1. 获取市场数据
	marketData, err := bot.fetchMarketData()
	if err != nil {
		return fmt.Errorf("获取市场数据失败: %w", err)
	}

	logger.Printf("%s当前价格: $%.2f", bot.config.Trading.SymbolA, marketData.Price)
	logger.Printf("数据周期: %s", bot.config.Trading.Timeframe)
	logger.Printf("价格变化: %+.2f%%", marketData.PriceChange)

	// 2. 获取当前持仓
	bot.currentPosition, err = bot.exchange.FetchPosition(bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB))
	if err != nil {
		logger.Printf("获取持仓失败: %v", err)
	} else if bot.currentPosition != nil {
		// 调试：打印持仓详细信息
		logger.Debugf("[DEBUG] 持仓详情 - 方向:%s, 数量:%.8f, 开仓价:%.2f, 未实现盈亏:%.2f USDT",
			bot.currentPosition.Side, bot.currentPosition.Size,
			bot.currentPosition.EntryPrice, bot.currentPosition.UnrealizedPnL)

		// 【修复】同步持仓信息到风险管理器
		if bot.riskManager != nil {
			bot.riskManager.UpdatePosition(bot.currentPosition)
		}
	} else {
		// 【修复】没有持仓时也要通知风险管理器
		if bot.riskManager != nil {
			bot.riskManager.UpdatePosition(nil)
		}
	}

	// 3. 获取账户USDT余额
	usdtBalance := 0.0
	balance, err := bot.exchange.FetchBalance(bot.config.Trading.SymbolB)
	if err != nil {
		logger.Printf("[WARNING] 获取%s余额失败: %v", bot.config.Trading.SymbolB, err)
	} else {
		usdtBalance = balance
	}

	// 4. AI分析生成交易信号 (使用交易对标识来隔离会话)
	signal, err := bot.aiClient.AnalyzeMarket(bot.tradingPair, marketData, bot.currentPosition, bot.config.Trading.SymbolA, usdtBalance)
	if err != nil {
		return fmt.Errorf("AI分析失败: %w", err)
	}

	// 注意: 信号历史现在由AI客户端内部管理，无需在Bot中维护

	// 5. 执行交易
	return bot.executeTrade(signal, marketData)
}

// fetchMarketData 获取市场数据并计算技术指标
func (bot *TradingBot) fetchMarketData() (*models.MarketData, error) {
	// 获取K线数据
	ohlcvList, err := bot.exchange.FetchOHLCV(
		bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB),
		bot.config.Trading.Timeframe,
		bot.config.Trading.DataPoints,
	)
	if err != nil {
		return nil, err
	}

	if len(ohlcvList) == 0 {
		return nil, fmt.Errorf("未获取到K线数据")
	}

	// 计算技术指标
	techData := bot.calculator.Calculate(ohlcvList)
	trendAnalysis := bot.calculator.CalculateTrendAnalysis(ohlcvList, techData)
	levelsAnalysis := bot.calculator.CalculateLevelsAnalysis(ohlcvList, techData)

	// 获取最新和上一根K线
	current := ohlcvList[len(ohlcvList)-1]
	previous := ohlcvList[len(ohlcvList)-2]

	// 构建市场数据
	marketData := &models.MarketData{
		Price:          current.Close,
		Timestamp:      time.Now().Format("2006-01-02 15:04:05"),
		High:           current.High,
		Low:            current.Low,
		Volume:         current.Volume,
		Timeframe:      bot.config.Trading.Timeframe,
		PriceChange:    ((current.Close - previous.Close) / previous.Close) * 100,
		KlineData:      ohlcvList,
		TechnicalData:  techData,
		TrendAnalysis:  trendAnalysis,
		LevelsAnalysis: levelsAnalysis,
	}

	return marketData, nil
}

// executeTrade 执行交易
func (bot *TradingBot) executeTrade(signal *models.TradeSignal, marketData *models.MarketData) error {
	// 获取当前会话的统计信息
	sessionInfo := bot.aiClient.GetSessionInfo(bot.tradingPair)
	statsStr := ""
	if sessionInfo != nil {
		statsStr = " " + sessionInfo.Stats.FormatStats()
	}

	logger.Printf("交易信号: %s%s", signal.Signal, statsStr)
	logger.Printf("信心程度: %s", signal.Confidence)
	logger.Printf("理由: %s", signal.Reason)

	// 风险管理：低信心信号不执行
	if signal.Confidence == "LOW" && !bot.config.Trading.TestMode {
		logger.Println("⚠️ 低信心信号，跳过执行")
		return nil
	}

	if bot.config.Trading.TestMode {
		logger.Println("测试模式 - 仅模拟交易")
		return nil
	}

	// HOLD信号不执行
	if signal.Signal == "HOLD" {
		logger.Println("建议观望，不执行交易")
		return nil
	}

	// 检查保证金并执行交易
	return bot.placeOrder(signal, marketData)
}

// placeOrder 下单
func (bot *TradingBot) placeOrder(signal *models.TradeSignal, marketData *models.MarketData) error {
	// amount配置现在是以symbolB为单位（如USDT），需要转换为symbolA数量（如BTC）
	// 例如: amount=1000 USDT, price=50000 USDT/BTC => amountInBase=1000/50000=0.02 BTC
	amountInBase := bot.config.Trading.Amount / marketData.Price

	// 根据交易模式选择不同的执行逻辑
	if bot.config.IsSpotMode() {
		// 现货模式：简单的买入/卖出
		return bot.executeSpotTrade(signal, amountInBase, marketData)
	} else {
		// 合约模式：开仓/平仓逻辑
		return bot.executeFuturesTrade(signal, amountInBase, marketData)
	}
}

// executeSpotTrade 执行现货交易
func (bot *TradingBot) executeSpotTrade(signal *models.TradeSignal, amountInBase float64, marketData *models.MarketData) error {
	logger.Printf("现货交易 - 金额: %.2f %s (约%.8f %s)",
		bot.config.Trading.Amount, bot.config.Trading.SymbolB,
		amountInBase, bot.config.Trading.SymbolA)

	if signal.Signal == "BUY" {
		// 检查USDT余额
		usdtBalance, err := bot.exchange.FetchBalance(bot.config.Trading.SymbolB)
		if err != nil {
			logger.Printf("[WARNING] 获取%s余额失败: %v，继续尝试下单", bot.config.Trading.SymbolB, err)
		} else {
			logger.Printf("[INFO] 当前%s可用余额: %.2f", bot.config.Trading.SymbolB, usdtBalance)
			if usdtBalance < bot.config.Trading.Amount {
				return fmt.Errorf("余额不足: 需要%.2f %s，但只有%.2f %s",
					bot.config.Trading.Amount, bot.config.Trading.SymbolB,
					usdtBalance, bot.config.Trading.SymbolB)
			}
		}

		logger.Println("执行买入...")
		err = bot.exchange.PlaceOrder(
			bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB),
			"buy",
			amountInBase,
			map[string]interface{}{},
		)
		if err != nil {
			return fmt.Errorf("买入失败: %w", err)
		}
		logger.Println("✅ 买入订单执行成功")

		// 等待订单成交并更新余额信息
		time.Sleep(2 * time.Second)
		btcBalance, err := bot.exchange.FetchBalance(bot.config.Trading.SymbolA)
		if err == nil {
			logger.Printf("[INFO] 买入后%s余额: %.8f", bot.config.Trading.SymbolA, btcBalance)
		}

	} else if signal.Signal == "SELL" {
		// 检查BTC余额
		btcBalance, err := bot.exchange.FetchBalance(bot.config.Trading.SymbolA)
		if err != nil {
			return fmt.Errorf("获取%s余额失败: %w", bot.config.Trading.SymbolA, err)
		}

		logger.Printf("[INFO] 当前%s可用余额: %.8f", bot.config.Trading.SymbolA, btcBalance)

		// 检查是否有足够的币可以卖出
		if btcBalance < amountInBase {
			// 如果余额不足但有余额，尝试卖出全部
			if btcBalance > 0 {
				logger.Printf("[WARNING] %s余额不足: 需要%.8f，但只有%.8f，将卖出全部余额",
					bot.config.Trading.SymbolA, amountInBase, btcBalance)
				amountInBase = btcBalance
			} else {
				logger.Printf("[ERROR] 没有%s可卖出，跳过本次交易", bot.config.Trading.SymbolA)
				return fmt.Errorf("没有%s可卖出，余额为0", bot.config.Trading.SymbolA)
			}
		}

		logger.Printf("执行卖出 %.8f %s...", amountInBase, bot.config.Trading.SymbolA)
		err = bot.exchange.PlaceOrder(
			bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB),
			"sell",
			amountInBase,
			map[string]interface{}{},
		)
		if err != nil {
			return fmt.Errorf("卖出失败: %w", err)
		}
		logger.Println("✅ 卖出订单执行成功")

		// 等待订单成交并更新余额信息
		time.Sleep(2 * time.Second)
		usdtBalance, err := bot.exchange.FetchBalance(bot.config.Trading.SymbolB)
		if err == nil {
			logger.Printf("[INFO] 卖出后%s余额: %.2f", bot.config.Trading.SymbolB, usdtBalance)
		}
	}

	return nil
}

// executeFuturesTrade 执行合约交易
func (bot *TradingBot) executeFuturesTrade(signal *models.TradeSignal, amountInBase float64, marketData *models.MarketData) error {
	// 根据信号确定操作类型和保证金需求
	var operationType string
	var requiredMargin float64

	if signal.Signal == "BUY" {
		if bot.currentPosition != nil && bot.currentPosition.Side == "short" {
			operationType = "平空开多"
			requiredMargin = bot.config.Trading.Amount / float64(bot.config.Trading.Leverage)
		} else if bot.currentPosition == nil {
			operationType = "开多仓"
			requiredMargin = bot.config.Trading.Amount / float64(bot.config.Trading.Leverage)
		} else {
			operationType = "保持多仓"
			requiredMargin = 0
		}
	} else if signal.Signal == "SELL" {
		if bot.currentPosition != nil && bot.currentPosition.Side == "long" {
			operationType = "平多开空"
			requiredMargin = bot.config.Trading.Amount / float64(bot.config.Trading.Leverage)
		} else if bot.currentPosition == nil {
			operationType = "开空仓"
			requiredMargin = bot.config.Trading.Amount / float64(bot.config.Trading.Leverage)
		} else {
			operationType = "保持空仓"
			requiredMargin = 0
		}
	}

	logger.Printf("操作类型: %s, 交易金额: %.2f %s (约%.8f %s), 需要保证金: %.2f %s",
		operationType, bot.config.Trading.Amount, bot.config.Trading.SymbolB,
		amountInBase, bot.config.Trading.SymbolA,
		requiredMargin, bot.config.Trading.SymbolB)

	// 执行交易逻辑
	if signal.Signal == "BUY" {
		return bot.executeBuy(signal, amountInBase)
	} else if signal.Signal == "SELL" {
		return bot.executeSell(signal, amountInBase)
	}

	return nil
}

// executeBuy 执行买入
func (bot *TradingBot) executeBuy(signal *models.TradeSignal, amountInBase float64) error {
	if bot.currentPosition != nil && bot.currentPosition.Side == "short" {
		// 平空仓
		logger.Println("平空仓...")
		err := bot.exchange.PlaceOrder(
			bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB),
			"buy",
			bot.currentPosition.Size,
			map[string]interface{}{
				"reduceOnly": true,
				"posSide":    "short", // 平空仓需要指定 posSide
			},
		)
		if err != nil {
			return fmt.Errorf("平空仓失败: %w", err)
		}
		time.Sleep(1 * time.Second)

		// 开多仓
		logger.Println("开多仓...")
		err = bot.exchange.PlaceOrder(
			bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB),
			"buy",
			amountInBase,
			map[string]interface{}{
				"posSide": "long", // 开多仓需要指定 posSide
			},
		)
		if err != nil {
			return fmt.Errorf("开多仓失败: %w", err)
		}
	} else if bot.currentPosition != nil && bot.currentPosition.Side == "long" {
		logger.Println("已有多头持仓，保持现状")
		logger.Printf("[INFO] 当前持仓: %.8f %s @ $%.2f, 未实现盈亏: %.2f USDT",
			bot.currentPosition.Size, bot.config.Trading.SymbolA,
			bot.currentPosition.EntryPrice, bot.currentPosition.UnrealizedPnL)
		logger.Println("[提示] 如需追加仓位，可考虑增加单次交易金额或使用独立的加仓策略")

		// 【修复】确保风险管理器知道当前持仓
		if bot.riskManager != nil {
			bot.riskManager.UpdatePosition(bot.currentPosition)
		}
		return nil
	} else {
		// 开多仓
		logger.Println("开多仓...")
		err := bot.exchange.PlaceOrder(
			bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB),
			"buy",
			amountInBase,
			map[string]interface{}{
				"posSide": "long", // 开多仓需要指定 posSide
			},
		)
		if err != nil {
			return fmt.Errorf("开多仓失败: %w", err)
		}
	}

	logger.Println("订单执行成功")
	time.Sleep(2 * time.Second)

	// 更新持仓
	pos, err := bot.exchange.FetchPosition(bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB))
	if err == nil {
		bot.currentPosition = pos
		logger.Printf("更新后持仓: %+v", pos)

		// 通知风险管理器更新持仓
		if bot.riskManager != nil {
			bot.riskManager.UpdatePosition(pos)
		}
	}

	// 获取并显示当前USDT余额
	usdtBalance, err := bot.exchange.FetchBalance(bot.config.Trading.SymbolB)
	if err == nil {
		logger.Printf("[INFO] 当前账户%s余额: %.2f", bot.config.Trading.SymbolB, usdtBalance)
	} else {
		logger.Printf("[WARNING] 获取%s余额失败: %v", bot.config.Trading.SymbolB, err)
	}

	return nil
}

// executeSell 执行卖出
func (bot *TradingBot) executeSell(signal *models.TradeSignal, amountInBase float64) error {
	if bot.currentPosition != nil && bot.currentPosition.Side == "long" {
		// 平多仓
		logger.Println("平多仓...")
		err := bot.exchange.PlaceOrder(
			bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB),
			"sell",
			bot.currentPosition.Size,
			map[string]interface{}{
				"reduceOnly": true,
				"posSide":    "long", // 平多仓需要指定 posSide
			},
		)
		if err != nil {
			return fmt.Errorf("平多仓失败: %w", err)
		}
		time.Sleep(1 * time.Second)

		// 开空仓
		logger.Println("开空仓...")
		err = bot.exchange.PlaceOrder(
			bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB),
			"sell",
			amountInBase,
			map[string]interface{}{
				"posSide": "short", // 开空仓需要指定 posSide
			},
		)
		if err != nil {
			return fmt.Errorf("开空仓失败: %w", err)
		}
	} else if bot.currentPosition != nil && bot.currentPosition.Side == "short" {
		logger.Println("已有空头持仓，保持现状")
		logger.Printf("[INFO] 当前持仓: %.8f %s @ $%.2f, 未实现盈亏: %.2f USDT",
			bot.currentPosition.Size, bot.config.Trading.SymbolA,
			bot.currentPosition.EntryPrice, bot.currentPosition.UnrealizedPnL)
		logger.Println("[提示] 如需追加仓位，可考虑增加单次交易金额或使用独立的加仓策略")

		// 【修复】确保风险管理器知道当前持仓
		if bot.riskManager != nil {
			bot.riskManager.UpdatePosition(bot.currentPosition)
		}
		return nil
	} else {
		// 开空仓
		logger.Println("开空仓...")
		err := bot.exchange.PlaceOrder(
			bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB),
			"sell",
			amountInBase,
			map[string]interface{}{
				"posSide": "short", // 开空仓需要指定 posSide
			},
		)
		if err != nil {
			return fmt.Errorf("开空仓失败: %w", err)
		}
	}

	logger.Println("订单执行成功")
	time.Sleep(2 * time.Second)

	// 更新持仓
	pos, err := bot.exchange.FetchPosition(bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB))
	if err == nil {
		bot.currentPosition = pos
		logger.Printf("更新后持仓: %+v", pos)

		// 通知风险管理器更新持仓
		if bot.riskManager != nil {
			bot.riskManager.UpdatePosition(pos)
		}
	}

	// 获取并显示当前USDT余额
	usdtBalance, err := bot.exchange.FetchBalance(bot.config.Trading.SymbolB)
	if err == nil {
		logger.Printf("[INFO] 当前账户%s余额: %.2f", bot.config.Trading.SymbolB, usdtBalance)
	} else {
		logger.Printf("[WARNING] 获取%s余额失败: %v", bot.config.Trading.SymbolB, err)
	}

	return nil
}

// SetupExchange 设置交易所参数
func (bot *TradingBot) SetupExchange() error {
	// 设置杠杆
	err := bot.exchange.SetLeverage(bot.exchange.ParseSymbols(bot.config.Trading.SymbolA, bot.config.Trading.SymbolB), bot.config.Trading.Leverage)
	if err != nil {
		return fmt.Errorf("设置杠杆失败: %w", err)
	}
	logger.Printf("设置杠杆倍数: %dx", bot.config.Trading.Leverage)

	return nil
}

// StartRiskManager 启动风险管理器
func (bot *TradingBot) StartRiskManager() error {
	if bot.riskManager != nil {
		return bot.riskManager.Start()
	}
	return nil
}

// StopRiskManager 停止风险管理器
func (bot *TradingBot) StopRiskManager() {
	if bot.riskManager != nil {
		bot.riskManager.Stop()
	}
}
