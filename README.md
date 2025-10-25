# DSBot

一个基于 Go 语言的加密货币交易机器人，支持 OKX 交易所。

## ⚠️ 风险提示

本项目仅供学习和研究使用。加密货币交易存在高风险，使用本软件进行实际交易可能导致资金损失。请务必：

- 充分理解代码逻辑后再使用
- 先在测试模式下运行
- 不要投入无法承受损失的资金
- 自行承担使用本软件的所有风险
- 作者仅测试过 BTC/USDT 交易对的合约交易可以正常运行, 其他交易对请小额尝试后再行决定!
- 不保证一定赚钱!
- 不保证一定赚钱!!
- 不保证一定赚钱!!!

## 功能特性

- ✅ 支持 OKX 交易所 (不确定是否会更新支持更多交易所)
- ✅ 支持现货和合约交易
- ✅ 技术指标分析 (RSI, MACD, 布林带等)
- ✅ AI 决策 (DeepSeek API)
- ✅ 风险管理 (止损、止盈、移动止损)
- ✅ 定时任务调度
- ✅ 完整的日志记录

## 快速开始

### 1. 环境要求

- Go 1.21 或更高版本
- OKX 交易所账户 (强烈建议创建子账户进行操作!)
- DeepSeek API Key

### 2. 安装

```bash
git clone https://github.com/crazycloudcc/dsbot.git
cd dsbot
go mod download
```

### 3. 配置

#### 方法一：使用配置文件（推荐用于开发）

```bash
# 复制示例配置文件
cp config.example.json config.json

# 编辑 config.json，填入你的 API 密钥
# 注意：config.json 已被 .gitignore 忽略，不会被提交到 git
```

#### 方法二：使用环境变量（推荐用于生产环境）

```bash
# Linux/Mac
export DEEPSEEK_API_KEY="your-deepseek-api-key"
export OKX_API_KEY="your-okx-api-key"
export OKX_SECRET="your-okx-secret"
export OKX_PASSWORD="your-okx-password"

# Windows PowerShell
$env:DEEPSEEK_API_KEY="your-deepseek-api-key"
$env:OKX_API_KEY="your-okx-api-key"
$env:OKX_SECRET="your-okx-secret"
$env:OKX_PASSWORD="your-okx-password"
```

环境变量会自动覆盖配置文件中的对应值，提供更高的安全性。

### 4. 运行

```bash
# 编译
go build -o dsbot ./cmd/api

# 运行 (使用默认配置文件 config.json)
./dsbot

# 或直接运行
go run ./cmd/api/main.go
```

## 配置说明

详细配置请参考 `config.example.json`：

- **trading**: 交易参数配置

  - `symbolA/symbolB`: 交易对(例如: BTC/USDT 交易对, symbolA 填 BTC, symbolB 填 USDT)
  - `amount`: 交易金额 (需要注意最小交易金额限制, 例如 BTC/USDT 合约最小金额通常需要 20USDT 以上)
  - `leverage`: 杠杆倍数（仅合约模式, 现货模式填 1）
  - `trading_mode`: 交易模式（spot/futures）
  - `risk_management`: 风险管理参数

- **api**: API 配置

  - `exchange_type`: 交易所类型（okx/binance）
  - DeepSeek API 配置
  - 交易所 API 密钥配置

- **logging**: 日志配置

## 项目结构

```
.
├── cmd/
│   └── api/
│       └── main.go           # 程序入口
├── internal/
│   ├── ai/                   # AI 决策模块
│   ├── config/               # 配置管理
│   ├── exchange/             # 交易所接口
│   ├── indicator/            # 技术指标计算
│   ├── logger/               # 日志模块
│   ├── models/               # 数据模型
│   ├── nets/                 # 网络请求
│   ├── strategy/             # 交易策略
│   └── timedschedulers/      # 定时任务
├── config.example.json       # 配置文件示例
└── README.md                 # 本文件
```

## 安全注意事项

⚠️ **重要：保护你的 API 密钥**

1. **永远不要**将包含真实 API 密钥的 `config.json` 提交到 Git
2. 使用环境变量存储敏感信息（推荐生产环境）
3. 定期轮换 API 密钥
4. 为交易 API 设置 IP 白名单
5. 使用只读 API 密钥进行测试
6. 限制 API 密钥的权限（只授予必要的交易权限）

## 开发

### 构建

```bash
go build -o dsbot ./cmd/api
```

### 测试

```bash
go test ./...
```

### 网络问题说明

默认没有设置代理, 如果需要配置请修改 internal/nets/http.go 中的 DefaultProxyURL, 不需要代理保持默认即可.

## License

MIT License

## 免责声明

使用本软件即表示您已阅读、理解并同意：

1. 本软件按"原样"提供，不提供任何明示或暗示的保证
2. 作者不对使用本软件造成的任何损失承担责任
3. 加密货币交易存在极高风险，请谨慎操作
4. 请遵守所在地区的法律法规

## 联系方式

如有问题或建议，请提交 Issue。
