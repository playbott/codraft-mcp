package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

func ParseLogLevel(str string) LogLevel {
	switch strings.ToUpper(strings.TrimSpace(str)) {
	case "DEBUG", "DBG", "0":
		return LevelDebug
	case "INFO", "INF", "1":
		return LevelInfo
	case "WARN", "WARNING", "WRN", "2":
		return LevelWarn
	case "ERROR", "ERR", "3":
		return LevelError
	default:
		return LevelInfo
	}
}

type Logger struct {
	mu    sync.RWMutex
	level LogLevel
	out   io.Writer
	file  *os.File
}

var globalLogger = &Logger{
	level: LevelInfo,
	out:   os.Stderr,
}

func init() {
	envLevel := os.Getenv("TRACKER_LOG_LEVEL")
	if envLevel == "" {
		envLevel = os.Getenv("LOG_LEVEL")
	}

	if envLevel != "" {
		globalLogger.level = ParseLogLevel(envLevel)
	}
}

func InitFileLogger(logFilePath string) {
	if globalLogger == nil {
		return
	}
	globalLogger.mu.Lock()
	defer globalLogger.mu.Unlock()

	if globalLogger.file != nil {
		globalLogger.file.Close()
		globalLogger.file = nil
	}

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file at %s: %v\n", logFilePath, err)
		return
	}

	globalLogger.file = logFile
	multi := io.MultiWriter(os.Stderr, logFile)
	globalLogger.out = multi
	log.SetOutput(multi)
	msg := fmt.Sprintf("%s [INFO ] [LOGGER] Log file initialized at %s\n",
		time.Now().Format("2006-01-02 15:04:05.000"), logFilePath)
	_, _ = fmt.Fprint(multi, msg)
	_ = logFile.Sync()
}

func CloseFileLogger() {
	if globalLogger == nil {
		return
	}
	globalLogger.mu.Lock()
	defer globalLogger.mu.Unlock()
	if globalLogger.file != nil {
		globalLogger.file.Close()
		globalLogger.file = nil
		globalLogger.out = os.Stderr
		log.SetOutput(os.Stderr)
	}
}

func SetLogLevel(level LogLevel) {
	if globalLogger == nil {
		return
	}
	globalLogger.mu.Lock()
	defer globalLogger.mu.Unlock()
	globalLogger.level = level
	fmt.Fprintf(globalLogger.out, "%s [INFO ] [LOGGER] Log level changed to %s\n",
		time.Now().Format("2006-01-02 15:04:05.000"), level.String())
}

func SetLogLevelString(str string) LogLevel {
	lvl := ParseLogLevel(str)
	SetLogLevel(lvl)
	return lvl
}

func GetLogLevel() LogLevel {
	if globalLogger == nil {
		return LevelInfo
	}
	globalLogger.mu.RLock()
	defer globalLogger.mu.RUnlock()
	return globalLogger.level
}

func logMessage(level LogLevel, component, format string, args ...interface{}) {
	if globalLogger == nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05.000")
		fmt.Fprintf(os.Stderr, "%s [%-5s] [%s] %s\n", timestamp, level.String(), component, fmt.Sprintf(format, args...))
		return
	}

	globalLogger.mu.RLock()
	currentLevel := globalLogger.level
	out := globalLogger.out
	globalLogger.mu.RUnlock()

	if level < currentLevel {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	if component != "" {
		fmt.Fprintf(out, "%s [%-5s] [%s] %s\n", timestamp, level.String(), component, msg)
	} else {
		fmt.Fprintf(out, "%s [%-5s] %s\n", timestamp, level.String(), msg)
	}
}

func LogDebug(component, format string, args ...interface{}) {
	logMessage(LevelDebug, component, format, args...)
}

func LogInfo(component, format string, args ...interface{}) {
	logMessage(LevelInfo, component, format, args...)
}

func LogWarn(component, format string, args ...interface{}) {
	logMessage(LevelWarn, component, format, args...)
}

func LogError(component, format string, args ...interface{}) {
	logMessage(LevelError, component, format, args...)
}
