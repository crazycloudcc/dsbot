package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dsbot/internal/ai"
	"dsbot/internal/config"
	"dsbot/internal/exchange"
	"dsbot/internal/logger"
	"dsbot/internal/strategy"
	"dsbot/internal/timedschedulers"

	"github.com/joho/godotenv"
)

func main() {
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		fmt.Println("未找到 .env 文件，将使用配置文件和系统环境变量")
	}

	// 加载配置
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志系统
	if cfg.Logging.EnableFileLogging {
		if err := logger.Init(
			cfg.Logging.LogDir,
			cfg.Logging.LogLevelConsole,
			cfg.Logging.LogLevelFile,
		); err != nil {
			fmt.Printf("初始化日志系统失败: %v\n", err)
			os.Exit(1)
		}
		defer logger.Close()
	} else {
		// 即使不启用文件日志，也初始化控制台日志（传入空字符串表示不创建文件）
		if err := logger.Init("", cfg.Logging.LogLevelConsole, "DEBUG"); err != nil {
			fmt.Printf("初始化日志系统失败: %v\n", err)
			os.Exit(1)
		}
	}

	// 初始化客户端
	tradingMode := cfg.GetTradingMode()
	exchangeClient, err := exchange.NewExchange(&cfg.API, tradingMode)
	if err != nil {
		logger.Printf("创建交易所客户端失败: %v", err)
		os.Exit(1)
	}
	deepseekClient := ai.NewDeepSeekClient(&cfg.API)

	// 创建交易机器人
	bot := strategy.NewTradingBot(cfg, exchangeClient, deepseekClient)

	// 打印启动信息
	printStartupInfo(cfg)

	// 设置交易所参数
	if err := bot.SetupExchange(); err != nil {
		logger.Printf("交易所设置失败: %v", err)
	}

	// 【修复】启动风险管理器前先获取当前持仓
	if cfg.IsFuturesMode() {
		symbol := exchangeClient.ParseSymbols(cfg.Trading.SymbolA, cfg.Trading.SymbolB)
		currentPos, err := exchangeClient.FetchPosition(symbol)
		if err != nil {
			logger.Printf("获取初始持仓失败: %v", err)
		} else if currentPos != nil {
			logger.Printf("[风险管理] 检测到已有持仓 - 方向:%s, 数量:%.8f, 开仓价:%.2f",
				currentPos.Side, currentPos.Size, currentPos.EntryPrice)
		}
	}

	// 启动风险管理器（如果已启用）
	if err := bot.StartRiskManager(); err != nil {
		logger.Printf("启动风险管理器失败: %v", err)
	}
	defer bot.StopRiskManager()

	// 创建交易任务调度器
	// 模式：config配置的时间+延迟3秒执行，立即执行一次
	var tradingScheduler *timedschedulers.Scheduler
	tradingScheduler = timedschedulers.NewScheduler(
		bot.Run,
		time.Duration(cfg.Trading.ScheduleIntervalMinutes)*time.Minute,
		timedschedulers.WithAlignedSchedule(3*time.Second),
		timedschedulers.WithRunImmediately(true),
		timedschedulers.WithErrorHandler(func(err error) {
			logger.Printf("执行交易失败: %v", err)
		}),
		timedschedulers.WithCompleteHandler(func() {
			nextRun := tradingScheduler.GetNextRunTime()
			logger.Printf("下次执行时间: %s", nextRun.Format("2006-01-02 15:04:05"))
		}),
	)

	// 创建日志轮转调度器（每小时执行一次）
	var logScheduler *timedschedulers.Scheduler
	if cfg.Logging.EnableFileLogging {
		logScheduler = timedschedulers.NewScheduler(
			func() error {
				return logger.RotateLog(cfg.Logging.LogDir)
			},
			time.Hour,
			timedschedulers.WithRunImmediately(false),
			timedschedulers.WithErrorHandler(func(err error) {
				logger.Printf("日志轮转失败: %v", err)
			}),
		)
	}

	// 启动调度器
	if err := tradingScheduler.Start(); err != nil {
		logger.Printf("启动交易调度器失败: %v", err)
		os.Exit(1)
	}
	defer tradingScheduler.Stop()

	if logScheduler != nil {
		if err := logScheduler.Start(); err != nil {
			logger.Printf("启动日志轮转调度器失败: %v", err)
		}
		defer logScheduler.Stop()
	}

	// 显示调度信息
	intervalMinutes := cfg.Trading.ScheduleIntervalMinutes
	alignPoints := calculateAlignPoints(intervalMinutes)
	logger.Printf("调度模式: 每小时 %v 分 + 延迟3秒执行", alignPoints)

	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n机器人正在运行中... 按 Ctrl+C 退出")

	// 等待退出信号
	<-sigChan
	fmt.Println("\n收到退出信号，正在停止机器人...")
	logger.Println("正在停止调度器...")
}

// calculateAlignPoints 计算对齐点（用于显示）
func calculateAlignPoints(intervalMinutes int) []int {
	var points []int
	for minute := 0; minute < 60; minute += intervalMinutes {
		points = append(points, minute)
	}
	return points
}

func printStartupInfo(cfg *config.Config) {
	exchangeType := cfg.API.ExchangeType

	logger.Println("============================================================")
	logger.Printf("%s/%s %s 自动交易机器人启动成功！", cfg.Trading.SymbolA, cfg.Trading.SymbolB, exchangeType)
	logger.Println("Go语言版本 - 融合技术指标策略 + 多交易所支持")
	logger.Println("============================================================")

	if cfg.Trading.TestMode {
		logger.Println("⚠️  当前为模拟模式，不会真实下单")
	} else {
		logger.Println("🔴 实盘交易模式，请谨慎操作！")
	}

	logger.Printf("交易所: %s", exchangeType)
	logger.Printf("交易对: %s/%s", cfg.Trading.SymbolA, cfg.Trading.SymbolB)
	logger.Printf("交易类型: %s", cfg.GetTradingMode())
	logger.Printf("交易周期: %s", cfg.Trading.Timeframe)
	logger.Printf("杠杆倍数: %dx", cfg.Trading.Leverage)
	logger.Printf("交易数量: %.8f %s", cfg.Trading.Amount, cfg.Trading.SymbolB)
	logger.Printf("执行频率: 每 %d 分钟", cfg.Trading.ScheduleIntervalMinutes)
	logger.Println("已启用完整技术指标分析和持仓跟踪功能")
	logger.Println("============================================================")
}
