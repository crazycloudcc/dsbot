package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	// DEBUG 调试级别
	DEBUG LogLevel = iota
	// INFO 信息级别
	INFO
	// WARN 警告级别
	WARN
	// ERROR 错误级别
	ERROR
	// FATAL 致命错误级别
	FATAL
)

// String 返回日志级别的字符串表示
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel 解析日志级别字符串
func ParseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO // 默认INFO级别
	}
}

var (
	// consoleLogger 控制台日志记录器
	consoleLogger *log.Logger
	// fileLogger 文件日志记录器
	fileLogger *log.Logger
	// logFile 日志文件句柄
	logFile *os.File
	// consoleLevelThreshold 控制台日志级别阈值
	consoleLevelThreshold LogLevel = INFO
	// fileLevelThreshold 文件日志级别阈值
	fileLevelThreshold LogLevel = DEBUG
)

// Init 初始化日志系统
func Init(logDir string, consoleLevel, fileLevel string) error {
	// 解析日志级别
	consoleLevelThreshold = ParseLogLevel(consoleLevel)
	fileLevelThreshold = ParseLogLevel(fileLevel)

	// 创建控制台日志记录器
	consoleLogger = log.New(os.Stdout, "", log.LstdFlags)

	// 如果启用文件日志
	if logDir != "" {
		// 创建日志目录
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %w", err)
		}

		// 生成日志文件名（按日期）
		now := time.Now()
		logFileName := fmt.Sprintf("trading_%s.log", now.Format("20060102"))
		logFilePath := filepath.Join(logDir, logFileName)

		// 打开或创建日志文件（追加模式）
		var err error
		logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("打开日志文件失败: %w", err)
		}

		// 创建文件日志记录器
		fileLogger = log.New(logFile, "", log.LstdFlags)

		// 写入启动日志
		fileLogger.Println("============================================================")
		fileLogger.Printf("日志系统初始化成功 - 日志文件: %s", logFilePath)
		fileLogger.Printf("控制台日志级别: %s, 文件日志级别: %s", consoleLevelThreshold, fileLevelThreshold)
		fileLogger.Println("============================================================")
	}

	consoleLogger.Println("============================================================")
	consoleLogger.Printf("日志系统初始化成功")
	consoleLogger.Printf("控制台日志级别: %s, 文件日志级别: %s", consoleLevelThreshold, fileLevelThreshold)
	consoleLogger.Println("============================================================")

	return nil
}

// Close 关闭日志文件
func Close() {
	if logFile != nil {
		if fileLogger != nil {
			fileLogger.Println("============================================================")
			fileLogger.Println("关闭日志系统")
			fileLogger.Println("============================================================")
		}
		logFile.Close()
	}
}

// shouldLog 检查是否应该记录该级别的日志
func shouldLogConsole(level LogLevel) bool {
	return level >= consoleLevelThreshold
}

func shouldLogFile(level LogLevel) bool {
	return level >= fileLevelThreshold
}

// logMessage 记录日志消息
func logMessage(level LogLevel, prefix string, v ...interface{}) {
	message := fmt.Sprint(v...)
	formattedMessage := fmt.Sprintf("[%s] %s", prefix, message)

	if shouldLogConsole(level) && consoleLogger != nil {
		consoleLogger.Println(formattedMessage)
	}

	if shouldLogFile(level) && fileLogger != nil {
		fileLogger.Println(formattedMessage)
	}
}

// logMessagef 格式化记录日志消息
func logMessagef(level LogLevel, prefix string, format string, v ...interface{}) {
	formattedMessage := fmt.Sprintf("[%s] "+format, append([]interface{}{prefix}, v...)...)

	if shouldLogConsole(level) && consoleLogger != nil {
		consoleLogger.Println(formattedMessage)
	}

	if shouldLogFile(level) && fileLogger != nil {
		fileLogger.Println(formattedMessage)
	}
}

// Printf 格式化输出日志（兼容旧代码，使用INFO级别）
func Printf(format string, v ...interface{}) {
	logMessagef(INFO, "INFO", format, v...)
}

// Println 输出日志行（兼容旧代码，使用INFO级别）
func Println(v ...interface{}) {
	logMessage(INFO, "INFO", v...)
}

// Fatalf 输出致命错误并退出
func Fatalf(format string, v ...interface{}) {
	logMessagef(FATAL, "FATAL", format, v...)
	os.Exit(1)
}

// Print 输出日志（兼容旧代码，使用INFO级别）
func Print(v ...interface{}) {
	logMessage(INFO, "INFO", v...)
}

// Info 输出信息日志
func Info(v ...interface{}) {
	logMessage(INFO, "INFO", v...)
}

// Infof 格式化输出信息日志
func Infof(format string, v ...interface{}) {
	logMessagef(INFO, "INFO", format, v...)
}

// Error 输出错误日志
func Error(v ...interface{}) {
	logMessage(ERROR, "ERROR", v...)
}

// Errorf 格式化输出错误日志
func Errorf(format string, v ...interface{}) {
	logMessagef(ERROR, "ERROR", format, v...)
}

// Warn 输出警告日志
func Warn(v ...interface{}) {
	logMessage(WARN, "WARN", v...)
}

// Warnf 格式化输出警告日志
func Warnf(format string, v ...interface{}) {
	logMessagef(WARN, "WARN", format, v...)
}

// Debug 输出调试日志
func Debug(v ...interface{}) {
	logMessage(DEBUG, "DEBUG", v...)
}

// Debugf 格式化输出调试日志
func Debugf(format string, v ...interface{}) {
	logMessagef(DEBUG, "DEBUG", format, v...)
}

// RotateLog 检查并轮转日志文件（按日期）
func RotateLog(logDir string) error {
	if logFile == nil {
		return nil // 没有文件日志，无需轮转
	}

	now := time.Now()
	logFileName := fmt.Sprintf("trading_%s.log", now.Format("20060102"))
	logFilePath := filepath.Join(logDir, logFileName)

	// 检查当前日志文件名是否需要更新
	currentLogPath := logFile.Name()
	if currentLogPath == logFilePath {
		// 日志文件名相同，无需轮转
		return nil
	}

	// 关闭旧文件
	logFile.Close()

	// 打开新日志文件
	var err error
	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("打开新日志文件失败: %w", err)
	}

	// 更新文件日志记录器
	fileLogger = log.New(logFile, "", log.LstdFlags)

	Infof("日志文件已轮转到: %s", logFilePath)

	return nil
}
