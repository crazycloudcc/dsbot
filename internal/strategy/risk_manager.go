package strategy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dsbot/internal/config"
	"dsbot/internal/exchange"
	"dsbot/internal/logger"
	"dsbot/internal/models"
)

// RiskManager 风险管理器（负责止盈止损监控）
type RiskManager struct {
	config          *config.Config
	exchange        exchange.Exchange
	tradingPair     string
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	running         bool
	mu              sync.Mutex
	currentPosition *models.Position
}

// NewRiskManager 创建风险管理器
func NewRiskManager(cfg *config.Config, exch exchange.Exchange, tradingPair string) *RiskManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &RiskManager{
		config:      cfg,
		exchange:    exch,
		tradingPair: tradingPair,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start 启动风险管理监控
func (rm *RiskManager) Start() error {
	rm.mu.Lock()
	if rm.running {
		rm.mu.Unlock()
		return fmt.Errorf("风险管理器已在运行中")
	}
	rm.running = true
	rm.mu.Unlock()

	logger.Println("[风险管理] 启动止盈止损监控...")
	logger.Printf("[风险管理] 止损: %.2f%%, 止盈: %.2f%%",
		rm.config.Trading.RiskManagement.StopLossPercent,
		rm.config.Trading.RiskManagement.TakeProfitPercent)

	if rm.config.Trading.RiskManagement.EnableTrailingStop {
		logger.Printf("[风险管理] 移动止损: 启用, 距离: %.2f%%",
			rm.config.Trading.RiskManagement.TrailingStopDistance)
	}

	// 【修复】启动时立即检查一次现有持仓
	go func() {
		time.Sleep(2 * time.Second) // 等待2秒确保系统完全启动
		rm.checkPosition()
	}()

	rm.wg.Add(1)
	go rm.monitorLoop()

	return nil
}

// Stop 停止风险管理监控
func (rm *RiskManager) Stop() {
	rm.mu.Lock()
	if !rm.running {
		rm.mu.Unlock()
		return
	}
	rm.mu.Unlock()

	logger.Println("[风险管理] 正在停止监控...")
	rm.cancel()
	rm.wg.Wait()

	rm.mu.Lock()
	rm.running = false
	rm.mu.Unlock()

	logger.Println("[风险管理] 监控已停止")
}

// IsRunning 检查是否正在运行
func (rm *RiskManager) IsRunning() bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.running
}

// UpdatePosition 更新当前持仓信息
func (rm *RiskManager) UpdatePosition(pos *models.Position) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if pos == nil {
		rm.currentPosition = nil
		logger.Debugf("[风险管理] 持仓已清空")
		return
	}

	// 如果是新开仓，计算止盈止损价格
	if rm.currentPosition == nil ||
		rm.currentPosition.EntryPrice != pos.EntryPrice ||
		rm.currentPosition.Side != pos.Side {
		rm.calculateStopLossTakeProfit(pos)
		logger.Printf("[风险管理] 新持仓监控开始 - 方向:%s, 开仓价:%.2f, 止损:%.2f, 止盈:%.2f",
			pos.Side, pos.EntryPrice, pos.StopLoss, pos.TakeProfit)
	}

	rm.currentPosition = pos
}

// calculateStopLossTakeProfit 计算止盈止损价格
func (rm *RiskManager) calculateStopLossTakeProfit(pos *models.Position) {
	cfg := rm.config.Trading.RiskManagement

	if pos.Side == "long" {
		// 多仓
		if cfg.EnableStopLoss {
			pos.StopLoss = pos.EntryPrice * (1 - cfg.StopLossPercent/100)
		}
		if cfg.EnableTakeProfit {
			pos.TakeProfit = pos.EntryPrice * (1 + cfg.TakeProfitPercent/100)
		}
		pos.HighestPrice = pos.EntryPrice
		pos.LowestPrice = pos.EntryPrice
	} else if pos.Side == "short" {
		// 空仓
		if cfg.EnableStopLoss {
			pos.StopLoss = pos.EntryPrice * (1 + cfg.StopLossPercent/100)
		}
		if cfg.EnableTakeProfit {
			pos.TakeProfit = pos.EntryPrice * (1 - cfg.TakeProfitPercent/100)
		}
		pos.HighestPrice = pos.EntryPrice
		pos.LowestPrice = pos.EntryPrice
	}

	// 初始化移动止损价格
	if cfg.EnableTrailingStop {
		// 【修复】如果固定止损未启用或为0，独立计算移动止损初始值
		if !cfg.EnableStopLoss || pos.StopLoss == 0 {
			if pos.Side == "long" {
				// 多仓：移动止损在开仓价下方
				pos.TrailingStop = pos.EntryPrice * (1 - cfg.TrailingStopDistance/100)
				logger.Printf("[风险管理] 移动止损独立初始化(多仓) - 开仓价:%.2f, 移动止损:%.2f",
					pos.EntryPrice, pos.TrailingStop)
			} else if pos.Side == "short" {
				// 空仓：移动止损在开仓价上方
				pos.TrailingStop = pos.EntryPrice * (1 + cfg.TrailingStopDistance/100)
				logger.Printf("[风险管理] 移动止损独立初始化(空仓) - 开仓价:%.2f, 移动止损:%.2f",
					pos.EntryPrice, pos.TrailingStop)
			}
		} else {
			// 使用固定止损作为移动止损初始值
			pos.TrailingStop = pos.StopLoss
			logger.Printf("[风险管理] 移动止损继承固定止损 - 止损价:%.2f", pos.TrailingStop)
		}
	}
}

// monitorLoop 监控循环
func (rm *RiskManager) monitorLoop() {
	defer rm.wg.Done()

	checkInterval := time.Duration(rm.config.Trading.RiskManagement.CheckIntervalSeconds) * time.Second
	if checkInterval == 0 {
		checkInterval = 10 * time.Second // 默认10秒
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.checkPosition()
		case <-rm.ctx.Done():
			return
		}
	}
}

// checkPosition 检查持仓并执行止盈止损
func (rm *RiskManager) checkPosition() {
	rm.mu.Lock()
	pos := rm.currentPosition
	rm.mu.Unlock()

	// 没有持仓，无需检查
	if pos == nil {
		return
	}

	// 获取当前价格
	symbol := rm.exchange.ParseSymbols(rm.config.Trading.SymbolA, rm.config.Trading.SymbolB)
	ticker, err := rm.exchange.FetchTicker(symbol)
	if err != nil {
		logger.Debugf("[风险管理] 获取价格失败: %v", err)
		return
	}

	currentPrice := ticker.Last

	// 【修复】增强调试日志 - 显示详细的止损状态
	rm.mu.Lock()
	logger.Debugf("[风险管理] 监控中 - 方向:%s, 当前价:%.2f, 开仓价:%.2f, 止损:%.2f, 止盈:%.2f, 移动止损:%.2f",
		pos.Side, currentPrice, pos.EntryPrice, pos.StopLoss, pos.TakeProfit, pos.TrailingStop)

	// 计算当前盈亏百分比（基于保证金）
	var currentPnL float64
	var pnlPercent float64
	if pos.Side == "long" {
		currentPnL = (currentPrice - pos.EntryPrice) * pos.Size
	} else {
		currentPnL = (pos.EntryPrice - currentPrice) * pos.Size
	}

	positionValue := pos.EntryPrice * pos.Size
	margin := positionValue / float64(pos.Leverage)
	if margin > 0 {
		pnlPercent = (currentPnL / margin) * 100
	}

	logger.Debugf("[风险管理] 当前浮动盈亏: %.2f USDT (%.2f%%), 止损阈值: %.2f%%",
		currentPnL/100, pnlPercent, rm.config.Trading.RiskManagement.StopLossPercent)
	rm.mu.Unlock()

	// 更新最高价和最低价
	rm.mu.Lock()
	if currentPrice > pos.HighestPrice {
		pos.HighestPrice = currentPrice
	}
	if currentPrice < pos.LowestPrice {
		pos.LowestPrice = currentPrice
	}
	rm.mu.Unlock()

	// 更新移动止损
	if rm.config.Trading.RiskManagement.EnableTrailingStop {
		rm.updateTrailingStop(pos, currentPrice)
	}

	// 检查是否触发止盈止损
	if rm.shouldClosePosition(pos, currentPrice) {
		rm.closePosition(pos, currentPrice)
	}
}

// updateTrailingStop 更新移动止损价格
func (rm *RiskManager) updateTrailingStop(pos *models.Position, currentPrice float64) {
	cfg := rm.config.Trading.RiskManagement
	trailingDistance := cfg.TrailingStopDistance / 100

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if pos.Side == "long" {
		// 多仓：价格上涨时，向上移动止损
		newTrailingStop := pos.HighestPrice * (1 - trailingDistance)
		if newTrailingStop > pos.TrailingStop {
			oldTrailing := pos.TrailingStop
			pos.TrailingStop = newTrailingStop
			logger.Printf("[风险管理] 移动止损更新 - 从 %.2f 调整到 %.2f (最高价: %.2f)",
				oldTrailing, newTrailingStop, pos.HighestPrice)
		}
	} else if pos.Side == "short" {
		// 空仓：价格下跌时，向下移动止损
		newTrailingStop := pos.LowestPrice * (1 + trailingDistance)
		if newTrailingStop < pos.TrailingStop {
			oldTrailing := pos.TrailingStop
			pos.TrailingStop = newTrailingStop
			logger.Printf("[风险管理] 移动止损更新 - 从 %.2f 调整到 %.2f (最低价: %.2f)",
				oldTrailing, newTrailingStop, pos.LowestPrice)
		}
	}
}

// shouldClosePosition 判断是否应该平仓
func (rm *RiskManager) shouldClosePosition(pos *models.Position, currentPrice float64) bool {
	cfg := rm.config.Trading.RiskManagement

	if pos.Side == "long" {
		// 多仓止损：价格跌破止损线
		if cfg.EnableStopLoss {
			// 优先检查移动止损（必须 > 0 才有效）
			if cfg.EnableTrailingStop && pos.TrailingStop > 0 && currentPrice <= pos.TrailingStop {
				logger.Printf("[风险管理] ⚠️ 触发移动止损 - 当前价:%.2f <= 移动止损:%.2f",
					currentPrice, pos.TrailingStop)
				return true
			}
			// 检查固定止损（必须 > 0 才有效）
			if pos.StopLoss > 0 && currentPrice <= pos.StopLoss {
				logger.Printf("[风险管理] ⚠️ 触发止损 - 当前价:%.2f <= 止损价:%.2f",
					currentPrice, pos.StopLoss)
				return true
			}
		}

		// 多仓止盈：价格涨破止盈线（必须 > 0 才有效）
		if cfg.EnableTakeProfit && pos.TakeProfit > 0 && currentPrice >= pos.TakeProfit {
			logger.Printf("[风险管理] ✅ 触发止盈 - 当前价:%.2f >= 止盈价:%.2f",
				currentPrice, pos.TakeProfit)
			return true
		}

	} else if pos.Side == "short" {
		// 空仓止损：价格涨破止损线
		if cfg.EnableStopLoss {
			// 优先检查移动止损（必须 > 0 才有效）
			if cfg.EnableTrailingStop && pos.TrailingStop > 0 && currentPrice >= pos.TrailingStop {
				logger.Printf("[风险管理] ⚠️ 触发移动止损 - 当前价:%.2f >= 移动止损:%.2f",
					currentPrice, pos.TrailingStop)
				return true
			}
			// 检查固定止损（必须 > 0 才有效）
			if pos.StopLoss > 0 && currentPrice >= pos.StopLoss {
				logger.Printf("[风险管理] ⚠️ 触发止损 - 当前价:%.2f >= 止损价:%.2f",
					currentPrice, pos.StopLoss)
				return true
			}
		}

		// 空仓止盈：价格跌破止盈线（必须 > 0 才有效）
		if cfg.EnableTakeProfit && pos.TakeProfit > 0 && currentPrice <= pos.TakeProfit {
			logger.Printf("[风险管理] ✅ 触发止盈 - 当前价:%.2f <= 止盈价:%.2f",
				currentPrice, pos.TakeProfit)
			return true
		}
	}

	return false
}

// closePosition 平仓
func (rm *RiskManager) closePosition(pos *models.Position, currentPrice float64) {
	logger.Printf("[风险管理] 正在平仓 - 方向:%s, 数量:%.8f, 开仓价:%.2f, 当前价:%.2f",
		pos.Side, pos.Size, pos.EntryPrice, currentPrice)

	symbol := rm.exchange.ParseSymbols(rm.config.Trading.SymbolA, rm.config.Trading.SymbolB)

	var side string
	var posSide string

	if pos.Side == "long" {
		side = "sell"
		posSide = "long"
	} else {
		side = "buy"
		posSide = "short"
	}

	// 执行平仓
	err := rm.exchange.PlaceOrder(
		symbol,
		side,
		pos.Size,
		map[string]interface{}{
			"reduceOnly": true,
			"posSide":    posSide,
		},
	)

	if err != nil {
		logger.Printf("[风险管理] ❌ 平仓失败: %v", err)
		return
	}

	// 计算盈亏
	var pnl float64
	var pnlPercent float64

	if pos.Side == "long" {
		pnl = (currentPrice - pos.EntryPrice) * pos.Size
	} else {
		pnl = (pos.EntryPrice - currentPrice) * pos.Size
	}

	// 计算保证金收益率
	positionValue := pos.EntryPrice * pos.Size
	margin := positionValue / float64(pos.Leverage)
	if margin > 0 {
		pnlPercent = (pnl / margin) * 100
	}

	logger.Printf("[风险管理] ✅ 平仓成功 - 盈亏: %.2f USDT (%.2f%%)", pnl/100, pnlPercent)

	// 获取最新余额
	time.Sleep(1 * time.Second)
	balance, err := rm.exchange.FetchBalance(rm.config.Trading.SymbolB)
	if err == nil {
		logger.Printf("[风险管理] 当前账户%s余额: %.2f", rm.config.Trading.SymbolB, balance)
	}

	// 清空持仓
	rm.mu.Lock()
	rm.currentPosition = nil
	rm.mu.Unlock()
}
