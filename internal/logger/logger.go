package logger

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	mu     sync.RWMutex
	global *zap.Logger
	sugar  *zap.SugaredLogger
)

// Config zap 日志配置。
type Config struct {
	// Level: debug | info | warn | error
	Level string `yaml:"level"`
	// Encoding: json | console
	Encoding string `yaml:"encoding"`
	// OutputPaths: stdout / stderr / 文件路径，逗号分隔或 YAML 列表在上层解析
	OutputPaths []string `yaml:"output_paths"`
	// ErrorOutputPaths 错误输出路径
	ErrorOutputPaths []string `yaml:"error_output_paths"`
}

func init() {
	// 保证配置加载前 Fatal/错误也有可用 logger
	l, err := newLogger(Config{Level: "info", Encoding: "console"})
	if err != nil {
		panic(fmt.Sprintf("init default logger: %v", err))
	}
	replaceGlobals(l)
}

// Init 按配置初始化全局 logger。
func Init(cfg Config) error {
	l, err := newLogger(cfg)
	if err != nil {
		return err
	}
	replaceGlobals(l)
	return nil
}

// L 返回全局 *zap.Logger。
func L() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return global
}

// S 返回全局 *zap.SugaredLogger，便于 printf 风格调用。
func S() *zap.SugaredLogger {
	mu.RLock()
	defer mu.RUnlock()
	return sugar
}

// Sync 刷新缓冲；进程退出前调用。忽略 stdout/stderr 的 sync 错误。
func Sync() {
	mu.RLock()
	l := global
	mu.RUnlock()
	if l == nil {
		return
	}
	_ = l.Sync()
}

// Named 返回带模块名的子 logger。
func Named(name string) *zap.Logger {
	return L().Named(name)
}

// NamedS 返回带模块名的 SugaredLogger。
func NamedS(name string) *zap.SugaredLogger {
	return L().Named(name).Sugar()
}

func replaceGlobals(l *zap.Logger) {
	mu.Lock()
	defer mu.Unlock()
	if global != nil {
		_ = global.Sync()
	}
	global = l
	sugar = l.Sugar()
	zap.ReplaceGlobals(l)
}

func newLogger(cfg Config) (*zap.Logger, error) {
	level := parseLevel(cfg.Level)
	encoding := strings.ToLower(strings.TrimSpace(cfg.Encoding))
	if encoding == "" {
		encoding = "console"
	}

	outputs := cfg.OutputPaths
	if len(outputs) == 0 {
		outputs = []string{"stdout"}
	}
	errOutputs := cfg.ErrorOutputPaths
	if len(errOutputs) == 0 {
		errOutputs = []string{"stderr"}
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.TimeKey = "ts"
	if encoding == "console" {
		encCfg = zap.NewDevelopmentEncoderConfig()
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	var encoder zapcore.Encoder
	if encoding == "json" {
		encoder = zapcore.NewJSONEncoder(encCfg)
	} else {
		encoder = zapcore.NewConsoleEncoder(encCfg)
	}

	sink, err := openSinks(outputs)
	if err != nil {
		return nil, err
	}
	errSink, err := openSinks(errOutputs)
	if err != nil {
		return nil, err
	}

	core := zapcore.NewCore(encoder, sink, level)
	opts := []zap.Option{
		zap.ErrorOutput(errSink),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	}
	return zap.New(core, opts...), nil
}

func parseLevel(s string) zap.AtomicLevel {
	lvl := zap.NewAtomicLevelAt(zap.InfoLevel)
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		lvl.SetLevel(zap.DebugLevel)
	case "info", "":
		lvl.SetLevel(zap.InfoLevel)
	case "warn", "warning":
		lvl.SetLevel(zap.WarnLevel)
	case "error":
		lvl.SetLevel(zap.ErrorLevel)
	case "fatal":
		lvl.SetLevel(zap.FatalLevel)
	default:
		lvl.SetLevel(zap.InfoLevel)
	}
	return lvl
}

func openSinks(paths []string) (zapcore.WriteSyncer, error) {
	writers := make([]zapcore.WriteSyncer, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		switch p {
		case "", "stdout":
			writers = append(writers, zapcore.AddSync(os.Stdout))
		case "stderr":
			writers = append(writers, zapcore.AddSync(os.Stderr))
		default:
			f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return nil, fmt.Errorf("open log file %s: %w", p, err)
			}
			writers = append(writers, zapcore.AddSync(f))
		}
	}
	if len(writers) == 1 {
		return writers[0], nil
	}
	return zap.CombineWriteSyncers(writers...), nil
}
