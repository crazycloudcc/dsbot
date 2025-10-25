package exchange

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dsbot/internal/config"
	"dsbot/internal/logger"
	"dsbot/internal/models"
	"dsbot/internal/nets"
)

const (
	OKXBaseURL = "https://www.okx.com"
)

// OKXClient OKX交易所客户端
type OKXClient struct {
	apiKey      string
	secret      string
	password    string
	httpClient  *nets.HttpClient
	tradingMode config.TradingMode // 交易模式
}

// NewOKXClient 创建OKX客户端
func NewOKXClient(cfg *config.APIConfig, tradingMode config.TradingMode) *OKXClient {
	_httpClient, err := nets.NewHttpClient(nets.DefaultTimeout, nets.DefaultProxyURL)
	if err != nil {
		fmt.Println("创建HTTP客户端失败:", err)
		return nil
	}

	return &OKXClient{
		apiKey:      cfg.OKXAPIKey,
		secret:      cfg.OKXSecret,
		password:    cfg.OKXPassword,
		httpClient:  _httpClient,
		tradingMode: tradingMode,
	}
}

// GetExchangeName 获取交易所名称
func (c *OKXClient) GetExchangeName() string {
	return string(config.ExchangeOKX)
}

func (c *OKXClient) ParseSymbols(symbolA, symbolB string) string {
	// BTC, USDT -> BTC/USDT:USDT
	return fmt.Sprintf("%s/%s:%s", symbolA, symbolB, symbolB)
}

// sign 生成签名
func (c *OKXClient) sign(timestamp, method, requestPath, body string) string {
	message := timestamp + method + requestPath + body
	h := hmac.New(sha256.New, []byte(c.secret))
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// request 发送HTTP请求
func (c *OKXClient) request(method, path string, body string) ([]byte, error) {
	url := OKXBaseURL + path
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	sign := c.sign(timestamp, method, path, body)

	headers := map[string]string{
		"OK-ACCESS-KEY":        c.apiKey,
		"OK-ACCESS-SIGN":       sign,
		"OK-ACCESS-TIMESTAMP":  timestamp,
		"OK-ACCESS-PASSPHRASE": c.password,
		"Content-Type":         "application/json",
	}

	switch method {
	case "GET":
		return c.httpClient.QueryGet(url, headers)
	case "POST":
		return c.httpClient.QueryPost(url, headers, []byte(body))
	}

	return nil, nil
}

// FetchOHLCV 获取K线数据
func (c *OKXClient) FetchOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error) {
	// 转换symbol格式: BTC/USDT:USDT -> BTC-USDT-SWAP
	instID := c.convertSymbol(symbol)

	path := fmt.Sprintf("/api/v5/market/candles?instId=%s&bar=%s&limit=%d", instID, timeframe, limit)

	data, err := c.request("GET", path, "")
	if err != nil {
		return nil, err
	}

	var response struct {
		Code string     `json:"code"`
		Msg  string     `json:"msg"`
		Data [][]string `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if response.Code != "0" {
		return nil, fmt.Errorf("OKX API错误: %s", response.Msg)
	}

	var ohlcvList []models.OHLCV
	for _, item := range response.Data {
		if len(item) < 6 {
			continue
		}

		timestamp, _ := strconv.ParseFloat(item[0], 64)
		open, _ := strconv.ParseFloat(item[1], 64)
		high, _ := strconv.ParseFloat(item[2], 64)
		low, _ := strconv.ParseFloat(item[3], 64)
		close, _ := strconv.ParseFloat(item[4], 64)
		volume, _ := strconv.ParseFloat(item[5], 64)

		ohlcvList = append(ohlcvList, models.OHLCV{
			Timestamp: time.Unix(int64(timestamp)/1000, 0),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
		})
	}

	// OKX返回的数据是倒序的，需要反转
	c.reverseOHLCV(ohlcvList)

	return ohlcvList, nil
}

// FetchTicker 获取最新行情（用于获取当前价格）
func (c *OKXClient) FetchTicker(symbol string) (*models.Ticker, error) {
	instID := c.convertSymbol(symbol)
	path := fmt.Sprintf("/api/v5/market/ticker?instId=%s", instID)

	data, err := c.request("GET", path, "")
	if err != nil {
		return nil, err
	}

	var response struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstID string `json:"instId"`
			Last   string `json:"last"`   // 最新成交价
			BidPx  string `json:"bidPx"`  // 买一价
			AskPx  string `json:"askPx"`  // 卖一价
			Vol24h string `json:"vol24h"` // 24小时成交量
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if response.Code != "0" {
		return nil, fmt.Errorf("OKX API错误: %s", response.Msg)
	}

	if len(response.Data) == 0 {
		return nil, fmt.Errorf("未获取到ticker数据")
	}

	ticker := response.Data[0]
	last, _ := strconv.ParseFloat(ticker.Last, 64)
	bid, _ := strconv.ParseFloat(ticker.BidPx, 64)
	ask, _ := strconv.ParseFloat(ticker.AskPx, 64)

	return &models.Ticker{
		Symbol: symbol,
		Last:   last,
		Bid:    bid,
		Ask:    ask,
	}, nil
}

// FetchPosition 获取持仓信息（仅用于合约模式）
func (c *OKXClient) FetchPosition(symbol string) (*models.Position, error) {
	instID := c.convertSymbol(symbol)
	path := fmt.Sprintf("/api/v5/account/positions?instId=%s", instID)

	data, err := c.request("GET", path, "")
	if err != nil {
		return nil, err
	}

	var response struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstID  string `json:"instId"`
			PosSide string `json:"posSide"`
			Pos     string `json:"pos"`
			AvgPx   string `json:"avgPx"`
			Upl     string `json:"upl"`
			Lever   string `json:"lever"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if response.Code != "0" {
		return nil, fmt.Errorf("OKX API错误: %s", response.Msg)
	}

	for _, pos := range response.Data {
		size, _ := strconv.ParseFloat(pos.Pos, 64)
		if size > 0 {
			entryPrice, _ := strconv.ParseFloat(pos.AvgPx, 64)
			upl, _ := strconv.ParseFloat(pos.Upl, 64)
			leverage, _ := strconv.ParseInt(pos.Lever, 10, 64)

			logger.Debugf("[DEBUG] FetchPosition - PosSide:%s, Size:%.8f, AvgPx:%.2f, Upl:%.2f",
				pos.PosSide, size, entryPrice, upl)

			return &models.Position{
				Side:          pos.PosSide,
				Size:          size,
				EntryPrice:    entryPrice,
				UnrealizedPnL: upl,
				Leverage:      int(leverage),
				Symbol:        symbol,
			}, nil
		}
	}

	return nil, nil
}

// FetchBalance 获取账户余额（用于现货模式）
func (c *OKXClient) FetchBalance(currency string) (float64, error) {
	path := "/api/v5/account/balance"

	data, err := c.request("GET", path, "")
	if err != nil {
		return 0, err
	}

	var response struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Details []struct {
				Ccy       string `json:"ccy"`       // 币种
				AvailBal  string `json:"availBal"`  // 可用余额
				FrozenBal string `json:"frozenBal"` // 冻结余额
				CashBal   string `json:"cashBal"`   // 现金余额
			} `json:"details"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return 0, err
	}

	if response.Code != "0" {
		return 0, fmt.Errorf("OKX API错误: %s", response.Msg)
	}

	if len(response.Data) == 0 {
		return 0, nil
	}

	// 查找指定币种的余额
	for _, detail := range response.Data[0].Details {
		if detail.Ccy == currency {
			availBal, _ := strconv.ParseFloat(detail.AvailBal, 64)
			return availBal, nil
		}
	}

	return 0, nil
}

// GetInstrumentInfo 获取交易对信息（现货或合约）
func (c *OKXClient) GetInstrumentInfo(symbol string) (*InstrumentInfo, error) {
	instID := c.convertSymbol(symbol)

	// 根据交易模式选择不同的 instType
	instType := "SWAP" // 默认合约
	if c.tradingMode == config.TradingModeSpot {
		instType = "SPOT"
	}

	path := fmt.Sprintf("/api/v5/public/instruments?instType=%s&instId=%s", instType, instID)

	data, err := c.request("GET", path, "")
	if err != nil {
		return nil, err
	}

	// 添加调试日志：查看原始响应
	logger.Debugf("[DEBUG] GetInstrumentInfo原始响应: %s", string(data))

	var response struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstID    string `json:"instId"`
			CtVal     string `json:"ctVal"`     // 合约面值（现货为空）
			CtMult    string `json:"ctMult"`    // 合约乘数（现货为空）
			LotSz     string `json:"lotSz"`     // 下单数量精度
			MinSz     string `json:"minSz"`     // 最小下单数量
			MinAmt    string `json:"minAmt"`    // 最小订单金额（现货专用，注意：OKX现货API可能不返回此字段）
			TickSz    string `json:"tickSz"`    // 下单价格精度
			MaxMktAmt string `json:"maxMktAmt"` // 最大市价单金额（可选，用于参考）
			MaxLmtAmt string `json:"maxLmtAmt"` // 最大限价单金额（可选，用于参考）
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if response.Code != "0" {
		return nil, fmt.Errorf("OKX API错误: %s", response.Msg)
	}

	if len(response.Data) == 0 {
		return nil, fmt.Errorf("未找到交易对信息")
	}

	info := response.Data[0]
	ctVal, _ := strconv.ParseFloat(info.CtVal, 64)
	lotSz, _ := strconv.ParseFloat(info.LotSz, 64)
	minSz, _ := strconv.ParseFloat(info.MinSz, 64)
	minAmt, _ := strconv.ParseFloat(info.MinAmt, 64)

	// 添加调试日志：查看解析结果
	logger.Debugf("[DEBUG] GetInstrumentInfo解析 - InstID:%s, LotSz:%s, MinSz:%s, MinAmt:'%s'(len=%d, parsed=%.2f)",
		info.InstID, info.LotSz, info.MinSz, info.MinAmt, len(info.MinAmt), minAmt)

	// ✅ 重要：OKX现货API不返回minAmt字段，需要使用默认值
	// 根据OKX实际要求和测试经验，现货交易的最小订单金额如下：
	if minAmt <= 0 && c.tradingMode == config.TradingModeSpot {
		// 根据交易对设置合理的默认值
		if instID == "BTC-USDT" || instID == "BTC-USDC" {
			minAmt = 15.0 // BTC现货最小订单金额15 USDT（基于OKX实际要求）
			logger.Printf("[INFO] OKX API未返回minAmt字段，使用BTC默认值: %.2f USDT", minAmt)
		} else if instID == "ETH-USDT" || instID == "ETH-USDC" {
			minAmt = 10.0 // ETH现货最小订单金额10 USDT（基于OKX实际要求）
			logger.Printf("[INFO] OKX API未返回minAmt字段，使用ETH默认值: %.2f USDT", minAmt)
		} else {
			minAmt = 5.0 // 其他币种默认5 USDT（保守估值）
			logger.Printf("[INFO] OKX API未返回minAmt字段，使用通用默认值: %.2f USDT", minAmt)
		}
	}

	return &InstrumentInfo{
		InstID:        info.InstID,
		ContractValue: ctVal, // 现货模式下为0
		LotSize:       lotSz,
		MinSize:       minSz,
		MinAmount:     minAmt, // 现货最小订单金额（使用默认值）
		TickSize:      0,
	}, nil
}

// PlaceOrder 下单（支持现货和合约）
func (c *OKXClient) PlaceOrder(symbol, side string, amount float64, params map[string]interface{}) error {
	instID := c.convertSymbol(symbol)

	// 获取交易对信息以确定正确的下单数量
	instInfo, err := c.GetInstrumentInfo(symbol)
	if err != nil {
		return fmt.Errorf("获取交易对信息失败: %w", err)
	}

	var orderSize float64

	if c.tradingMode == config.TradingModeSpot {
		// 现货模式：amount 就是实际数量（BTC数量）
		orderSize = amount

		// 确保数量符合lotSize要求
		if instInfo.LotSize > 0 {
			orderSize = c.roundToLotSize(orderSize, instInfo.LotSize)
		}

		// 确保不小于最小下单数量
		if orderSize < instInfo.MinSize {
			orderSize = instInfo.MinSize
		}

		// 获取当前市场价格来检查最小订单金额
		if instInfo.MinAmount > 0 {
			// 获取ticker获取当前价格
			ticker, err := c.FetchTicker(symbol)
			if err == nil && ticker.Last > 0 {
				orderAmount := orderSize * ticker.Last // 订单金额（USDT）
				if orderAmount < instInfo.MinAmount {
					// 订单金额不足，需要调整数量
					requiredSize := instInfo.MinAmount / ticker.Last
					// 向上取整到lotSize
					if instInfo.LotSize > 0 {
						requiredSize = c.roundUpToLotSize(requiredSize, instInfo.LotSize)
					}
					logger.Printf("[WARNING] 订单金额%.2f不足最小要求%.2f，调整数量从%.8f到%.8f",
						orderAmount, instInfo.MinAmount, orderSize, requiredSize)
					orderSize = requiredSize
				}
			}
		}

		logger.Debugf("[DEBUG] 现货下单 - LotSize:%.8f, MinSize:%.8f, MinAmount:%.2f, 数量:%.8f",
			instInfo.LotSize, instInfo.MinSize, instInfo.MinAmount, orderSize)
	} else {
		// 合约模式：需要转换为张数
		var contractSize float64
		if instInfo.ContractValue > 0 {
			// amount是BTC数量，转换为张数
			// 例如：amount=0.00018101 BTC, ctVal=0.01 BTC/张
			// contractSize = 0.00018101 / 0.01 = 0.018101 张
			contractSize = amount / instInfo.ContractValue
		} else {
			contractSize = amount
		}

		logger.Debugf("[DEBUG] 合约计算 - amount:%.8f BTC, ctVal:%.4f BTC/张, 初始张数:%.4f",
			amount, instInfo.ContractValue, contractSize)

		// 确保数量符合lotSize要求（合约的lotSize是张数精度，如0.01张）
		if instInfo.LotSize > 0 {
			contractSize = c.roundToLotSize(contractSize, instInfo.LotSize)
			logger.Debugf("[DEBUG] 合约对齐 - lotSize:%.4f, 对齐后张数:%.4f", instInfo.LotSize, contractSize)
		}

		// 确保不小于最小下单数量（合约的minSize是最小张数，如0.01张）
		if contractSize < instInfo.MinSize {
			logger.Debugf("[DEBUG] 合约调整 - 张数%.4f < 最小值%.4f, 调整到最小值", contractSize, instInfo.MinSize)
			contractSize = instInfo.MinSize
		}

		orderSize = contractSize

		logger.Debugf("[DEBUG] 合约下单 - 面值:%.4f BTC/张, LotSize:%.4f张, MinSize:%.4f张, 最终张数:%.4f",
			instInfo.ContractValue, instInfo.LotSize, instInfo.MinSize, contractSize)
	}

	// 构建订单参数
	orderData := map[string]interface{}{
		"instId":  instID,
		"side":    side,
		"ordType": "market",
	}

	// 根据交易模式设置不同的参数
	if c.tradingMode == config.TradingModeSpot {
		// 现货交易参数
		orderData["tdMode"] = "cash"                     // 现货使用 cash 模式
		orderData["sz"] = fmt.Sprintf("%.8f", orderSize) // 现货使用8位小数
	} else {
		// 合约交易参数
		orderData["tdMode"] = "cross" // 合约使用 cross 或 isolated

		// ✅ 重要：合约张数不一定是整数！
		// lotSize=1时是整数（如ETH-USDT-SWAP），lotSize=0.01时是小数（如BTC-USDT-SWAP）
		// 根据lotSize决定精度
		var szFormat string
		if instInfo.LotSize >= 1 {
			szFormat = "%.0f" // 整数张数
		} else if instInfo.LotSize >= 0.1 {
			szFormat = "%.1f" // 1位小数
		} else if instInfo.LotSize >= 0.01 {
			szFormat = "%.2f" // 2位小数
		} else {
			szFormat = "%.4f" // 4位小数（保险起见）
		}
		orderData["sz"] = fmt.Sprintf(szFormat, orderSize)

		logger.Debugf("[DEBUG] 合约sz格式 - lotSize:%.4f, 格式:%s, orderSize:%.4f, sz:%s",
			instInfo.LotSize, szFormat, orderSize, fmt.Sprintf(szFormat, orderSize))
	}

	// 合并额外参数（如 posSide, reduceOnly 等，仅合约有效）
	for k, v := range params {
		orderData[k] = v
	}

	bodyBytes, err := json.Marshal(orderData)
	if err != nil {
		return err
	}

	// 记录请求详情
	logger.Debugf("[DEBUG] OKX下单请求: %s", string(bodyBytes))

	data, err := c.request("POST", "/api/v5/trade/order", string(bodyBytes))
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}

	// 记录响应详情
	logger.Debugf("[DEBUG] OKX响应: %s", string(data))

	var response struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			OrdId   string `json:"ordId"`
			ClOrdId string `json:"clOrdId"`
			SCode   string `json:"sCode"`
			SMsg    string `json:"sMsg"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return fmt.Errorf("解析响应失败: %w, 原始响应: %s", err, string(data))
	}

	if response.Code != "0" {
		// 如果有详细错误信息，显示出来
		if len(response.Data) > 0 && response.Data[0].SMsg != "" {
			return fmt.Errorf("OKX下单失败 [%s]: %s (详情: %s)", response.Code, response.Msg, response.Data[0].SMsg)
		}
		return fmt.Errorf("OKX下单失败 [%s]: %s", response.Code, response.Msg)
	}

	return nil
}

// SetLeverage 设置杠杆
func (c *OKXClient) SetLeverage(symbol string, leverage int) error {
	instID := c.convertSymbol(symbol)

	leverageData := map[string]interface{}{
		"instId":  instID,
		"lever":   fmt.Sprintf("%d", leverage),
		"mgnMode": "cross",
	}

	bodyBytes, err := json.Marshal(leverageData)
	if err != nil {
		return err
	}

	data, err := c.request("POST", "/api/v5/account/set-leverage", string(bodyBytes))
	if err != nil {
		return err
	}

	var response struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}

	if response.Code != "0" {
		return fmt.Errorf("设置杠杆失败: %s", response.Msg)
	}

	return nil
}

// 辅助函数

// roundToLotSize 将数量四舍五入到lotSize的整数倍
func (c *OKXClient) roundToLotSize(size, lotSize float64) float64 {
	if lotSize <= 0 {
		return size
	}
	return float64(int(size/lotSize+0.5)) * lotSize
}

// roundUpToLotSize 向上取整到lotSize的整数倍
func (c *OKXClient) roundUpToLotSize(size, lotSize float64) float64 {
	if lotSize <= 0 {
		return size
	}
	return float64(int(size/lotSize)+1) * lotSize
}

func (c *OKXClient) convertSymbol(symbol string) string {
	// BTC/USDT:USDT -> BTC-USDT (spot) or BTC-USDT-SWAP (futures)
	parts := strings.Split(symbol, "/")
	if len(parts) != 2 {
		return symbol
	}
	base := parts[0]
	quoteParts := strings.Split(parts[1], ":")
	if len(quoteParts) > 0 {
		quote := quoteParts[0]
		if c.tradingMode == config.TradingModeSpot {
			return base + "-" + quote
		}
		return base + "-" + quote + "-SWAP"
	}
	return symbol
}

func (c *OKXClient) reverseOHLCV(data []models.OHLCV) {
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
}
