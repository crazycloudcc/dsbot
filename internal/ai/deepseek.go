package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"dsbot/internal/config"
	"dsbot/internal/logger"
	"dsbot/internal/models"
	"dsbot/internal/nets"
)

// DeepSeekClient DeepSeek客户端
type DeepSeekClient struct {
	apiKey     string
	baseURL    string
	httpClient *nets.HttpClient
	sessions   map[string]*models.SessionContext // 多交易对会话上下文管理
}

// NewDeepSeekClient 创建DeepSeek客户端
func NewDeepSeekClient(cfg *config.APIConfig) *DeepSeekClient {
	_httpClient, err := nets.NewHttpClient(nets.DefaultTimeout, nets.DefaultProxyURL)
	if err != nil {
		fmt.Println("创建HTTP客户端失败:", err)
		return nil
	}

	return &DeepSeekClient{
		apiKey:     cfg.DeepSeekAPIKey,
		baseURL:    cfg.DeepSeekBaseURL,
		httpClient: _httpClient,
		sessions:   make(map[string]*models.SessionContext), // 初始化会话上下文映射
	}
}

// ChatRequest DeepSeek聊天请求
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	Stream      bool      `json:"stream"`
}

// Message 消息结构
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse DeepSeek响应
type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// AnalyzeMarket 分析市场并生成交易信号
func (c *DeepSeekClient) AnalyzeMarket(tradingPair string, marketData *models.MarketData, currentPosition *models.Position, symbolA string, usdtBalance float64) (*models.TradeSignal, error) {
	// 获取或创建该交易对的会话上下文
	session := c.getOrCreateSession(tradingPair)

	// 构建分析提示词 (使用该交易对的历史信号)
	prompt := c.buildAnalysisPrompt(tradingPair, marketData, currentPosition, session.SignalHistory, symbolA, usdtBalance)
	logger.Debugf("[%s] prompt: %s", tradingPair, prompt)

	// 调用DeepSeek API
	request := ChatRequest{
		Model: "deepseek-chat",
		Messages: []Message{
			{
				Role:    "system",
				Content: fmt.Sprintf("您是一位专业的加密货币交易员，专注于%s交易对的%s周期趋势分析。请结合K线形态和技术指标做出判断，并严格遵循JSON格式要求。注意：这是%s交易对的独立分析，不要混淆其他交易对的信息。", tradingPair, marketData.Timeframe, tradingPair),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.1,
		Stream:      false,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + c.apiKey,
	}

	body, err := c.httpClient.QueryPost(c.baseURL+"/v1/chat/completions", headers, requestBody)
	if err != nil {
		return nil, err
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, err
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("DeepSeek返回空响应")
	}

	content := chatResp.Choices[0].Message.Content
	logger.Infof("[%s] DeepSeek原始回复: %s", tradingPair, content)

	// 解析JSON响应
	signal, err := c.parseSignal(content, marketData)
	if err != nil {
		logger.Errorf("[%s] 解析信号失败，使用备用方案: %v", tradingPair, err)
		return c.createFallbackSignal(tradingPair, marketData), nil
	}

	signal.Timestamp = time.Now().Format("2006-01-02 15:04:05")
	signal.TradingPair = tradingPair

	// 更新该交易对的会话上下文
	c.updateSession(tradingPair, signal)

	return signal, nil
}

// getOrCreateSession 获取或创建交易对的会话上下文
func (c *DeepSeekClient) getOrCreateSession(tradingPair string) *models.SessionContext {
	if session, exists := c.sessions[tradingPair]; exists {
		return session
	}

	// 创建新的会话上下文
	session := &models.SessionContext{
		TradingPair:   tradingPair,
		SignalHistory: make([]models.TradeSignal, 0),
		LastUpdate:    time.Now().Format("2006-01-02 15:04:05"),
	}
	c.sessions[tradingPair] = session
	logger.Infof("为交易对 [%s] 创建新的AI会话上下文", tradingPair)
	return session
}

// updateSession 更新交易对的会话上下文
func (c *DeepSeekClient) updateSession(tradingPair string, signal *models.TradeSignal) {
	session := c.sessions[tradingPair]
	session.SignalHistory = append(session.SignalHistory, *signal)

	// 更新统计信息
	session.Stats.Total++
	switch signal.Signal {
	case "BUY":
		session.Stats.BuyCount++
	case "SELL":
		session.Stats.SellCount++
	case "HOLD":
		session.Stats.HoldCount++
	}

	// 限制历史记录数量 (每个交易对独立维护30条)
	if len(session.SignalHistory) > 30 {
		session.SignalHistory = session.SignalHistory[1:]
	}

	session.LastUpdate = signal.Timestamp
	logger.Debugf("[%s] 会话上下文已更新，历史信号数: %d", tradingPair, len(session.SignalHistory))
}

// GetSessionInfo 获取交易对的会话信息 (用于调试和监控)
func (c *DeepSeekClient) GetSessionInfo(tradingPair string) *models.SessionContext {
	if session, exists := c.sessions[tradingPair]; exists {
		return session
	}
	return nil
}

// buildAnalysisPrompt 构建分析提示词
func (c *DeepSeekClient) buildAnalysisPrompt(tradingPair string, marketData *models.MarketData, currentPosition *models.Position, signalHistory []models.TradeSignal, symbolA string, usdtBalance float64) string {
	// K线数据文本
	klineText := fmt.Sprintf("【最近5根%s K线数据】\n", marketData.Timeframe)
	if len(marketData.KlineData) > 0 {
		start := len(marketData.KlineData) - 5
		if start < 0 {
			start = 0
		}
		for i, kline := range marketData.KlineData[start:] {
			trend := "阳线"
			if kline.Close < kline.Open {
				trend = "阴线"
			}
			change := ((kline.Close - kline.Open) / kline.Open) * 100
			klineText += fmt.Sprintf("K线%d: %s 开盘:%.2f 收盘:%.2f 涨跌:%+.2f%%\n", i+1, trend, kline.Open, kline.Close, change)
		}
	}

	// 技术指标分析
	tech := marketData.TechnicalData
	techText := ""
	if tech != nil {
		techText = fmt.Sprintf(`
【技术指标分析】
📈 移动平均线:
- 5周期: %.2f | 价格相对: %+.2f%%
- 20周期: %.2f | 价格相对: %+.2f%%
- 50周期: %.2f | 价格相对: %+.2f%%

🎯 趋势分析:
- 短期趋势: %s
- 中期趋势: %s
- 整体趋势: %s
- MACD方向: %s

📊 动量指标:
- RSI: %.2f (%s)
- MACD: %.4f
- 信号线: %.4f

🎚️ 布林带位置: %.2f%% (%s)
`,
			tech.SMA5, (marketData.Price-tech.SMA5)/tech.SMA5*100,
			tech.SMA20, (marketData.Price-tech.SMA20)/tech.SMA20*100,
			tech.SMA50, (marketData.Price-tech.SMA50)/tech.SMA50*100,
			marketData.TrendAnalysis.ShortTerm,
			marketData.TrendAnalysis.MediumTerm,
			marketData.TrendAnalysis.Overall,
			marketData.TrendAnalysis.MACD,
			tech.RSI, getRSILevel(tech.RSI),
			tech.MACD,
			tech.MACDSignal,
			tech.BBPosition*100, getBBLevel(tech.BBPosition),
		)
	}

	// 持仓信息
	positionText := "无持仓"
	if currentPosition != nil {
		// 计算盈亏百分比（基于实际保证金）
		// 保证金收益率 = 未实现盈亏 / 实际保证金 × 100%
		// 实际保证金 = (开仓价格 × 持仓数量) / 杠杆倍数
		pnlPercentage := 0.0
		if currentPosition.EntryPrice > 0 && currentPosition.Size > 0 && currentPosition.Leverage > 0 {
			// 计算实际投入的保证金
			positionValue := currentPosition.EntryPrice * currentPosition.Size
			actualMargin := positionValue / float64(currentPosition.Leverage)

			// 基于保证金计算收益率
			if actualMargin > 0 {
				pnlPercentage = (currentPosition.UnrealizedPnL * 100 / actualMargin) * 100
			}
		}
		positionText = fmt.Sprintf("%s仓, 数量: %.8f, 盈亏: %.2fUSDT (%.2f%%)",
			currentPosition.Side, currentPosition.Size, currentPosition.UnrealizedPnL, pnlPercentage)
	}

	// 历史信号 (仅显示该交易对的最近信号)
	signalText := "- 首次分析, 无历史信号"
	if len(signalHistory) > 0 {
		lastSignal := signalHistory[len(signalHistory)-1]
		signalText = fmt.Sprintf("\n信号: %s\n信心: %s\n理由: %s", lastSignal.Signal, lastSignal.Confidence, lastSignal.Reason)
	}

	prompt := fmt.Sprintf(`
您是一位专业的加密货币交易员。基于当前市场数据和技术指标，
遵循以下规则决策：
1. 趋势明确时果断入场
2. 逆势持仓立即平仓

请基于以下%s %s周期数据进行分析：

%s

%s

【上次交易信号】
%s

【当前行情】
- 交易对: %s
- 当前价格: $%.2f
- 时间: %s
- 本K线最高: $%.2f
- 本K线最低: $%.2f
- 本K线成交量: %.2f %s
- 价格变化: %+.2f%%
- 当前持仓: %s
- 账户余额: %.2f USDT

【分析要求】
1. 基于%sK线趋势和技术指标给出交易信号: BUY(买入) / SELL(卖出) / HOLD(观望)
2. 简要分析理由（考虑趋势连续性、支撑阻力、成交量等因素）
3. 评估信号信心程度

【重要提示】
- 这是%s交易对的独立分析
- 不要混淆其他交易对的数据和历史
- 专注于当前交易对的市场状况

【重要格式要求】
- 必须返回纯JSON格式，不要有任何额外文本
- 所有属性名必须使用双引号
- 不要使用单引号
- 不要添加注释
- 确保JSON格式完全正确

请用以下JSON格式回复：
{
    "signal": "BUY|SELL|HOLD",
    "reason": "分析理由",
    "confidence": "HIGH|MEDIUM|LOW"
}
`,
		tradingPair,
		marketData.Timeframe,
		klineText,
		techText,
		signalText,
		tradingPair, // 在多处强调交易对
		marketData.Price,
		marketData.Timestamp,
		marketData.High,
		marketData.Low,
		marketData.Volume,
		symbolA,
		marketData.PriceChange,
		positionText,
		usdtBalance, // USDT余额
		marketData.Timeframe,
		tradingPair, // 再次强调
	)

	return prompt
}

// parseSignal 解析交易信号
func (c *DeepSeekClient) parseSignal(content string, marketData *models.MarketData) (*models.TradeSignal, error) {
	// 提取JSON部分 - 支持多行JSON
	re := regexp.MustCompile(`(?s)\{[^{}]*\}`)
	matches := re.FindString(content)
	if matches == "" {
		return nil, fmt.Errorf("未找到JSON格式数据")
	}

	// 清理和修复JSON
	jsonStr := strings.TrimSpace(matches)
	logger.Debugf("提取的JSON: %s", jsonStr)

	var signal models.TradeSignal
	if err := json.Unmarshal([]byte(jsonStr), &signal); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}

	// 验证必需字段
	if signal.Signal == "" {
		return nil, fmt.Errorf("信号字段为空")
	}
	if signal.Reason == "" {
		return nil, fmt.Errorf("理由字段为空")
	}

	// 记录解析结果
	logger.Debugf("解析成功 - 信号:%s, 信心:%s",
		signal.Signal, signal.Confidence)

	return &signal, nil
}

// createFallbackSignal 创建备用信号
func (c *DeepSeekClient) createFallbackSignal(tradingPair string, marketData *models.MarketData) *models.TradeSignal {
	return &models.TradeSignal{
		Signal:      "HOLD",
		Reason:      "因技术分析暂时不可用，采取保守策略",
		Confidence:  "LOW",
		Timestamp:   time.Now().Format("2006-01-02 15:04:05"),
		IsFallback:  true,
		TradingPair: tradingPair,
	}
}

// 辅助函数
func getRSILevel(rsi float64) string {
	if rsi > 70 {
		return "超买"
	} else if rsi < 30 {
		return "超卖"
	}
	return "中性"
}

func getBBLevel(position float64) string {
	if position > 0.7 {
		return "上部"
	} else if position < 0.3 {
		return "下部"
	}
	return "中部"
}
