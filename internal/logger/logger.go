package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	mu      sync.RWMutex
	global  *zap.Logger
	sugar   *zap.SugaredLogger
	closers []io.Closer
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
	// RotateDaily 为 true 时，文件输出按本地日期轮转（stdout/stderr 不受影响）
	RotateDaily bool `yaml:"rotate_daily"`
	// MaxAgeDays 归档保留天数；超过的按日期归档会被删除。<=0 表示不删除
	MaxAgeDays int `yaml:"max_age_days"`
}

func init() {
	// 保证配置加载前 Fatal/错误也有可用 logger
	l, cs, err := newLogger(Config{Level: "info", Encoding: "console"})
	if err != nil {
		panic(fmt.Sprintf("init default logger: %v", err))
	}
	replaceGlobals(l, cs)
}

// Init 按配置初始化全局 logger。
func Init(cfg Config) error {
	l, cs, err := newLogger(cfg)
	if err != nil {
		return err
	}
	replaceGlobals(l, cs)
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

func replaceGlobals(l *zap.Logger, cs []io.Closer) {
	mu.Lock()
	old := closers
	if global != nil {
		_ = global.Sync()
	}
	global = l
	sugar = l.Sugar()
	closers = cs
	mu.Unlock()
	zap.ReplaceGlobals(l)
	for _, c := range old {
		_ = c.Close()
	}
}

func newLogger(cfg Config) (*zap.Logger, []io.Closer, error) {
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

	sink, sinkClosers, err := openSinks(outputs, cfg.RotateDaily, cfg.MaxAgeDays)
	if err != nil {
		return nil, nil, err
	}
	errSink, errClosers, err := openSinks(errOutputs, cfg.RotateDaily, cfg.MaxAgeDays)
	if err != nil {
		closeAll(sinkClosers)
		return nil, nil, err
	}

	core := zapcore.NewCore(encoder, sink, level)
	opts := []zap.Option{
		zap.ErrorOutput(errSink),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	}
	return zap.New(core, opts...), append(sinkClosers, errClosers...), nil
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

func openSinks(paths []string, rotateDaily bool, maxAgeDays int) (zapcore.WriteSyncer, []io.Closer, error) {
	writers := make([]zapcore.WriteSyncer, 0, len(paths))
	var opened []io.Closer
	fail := func(err error) (zapcore.WriteSyncer, []io.Closer, error) {
		closeAll(opened)
		return nil, nil, err
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		switch p {
		case "", "stdout":
			writers = append(writers, zapcore.AddSync(os.Stdout))
		case "stderr":
			writers = append(writers, zapcore.AddSync(os.Stderr))
		default:
			if rotateDaily {
				dw, err := openDailyWriter(p, maxAgeDays, nil)
				if err != nil {
					return fail(err)
				}
				writers = append(writers, dw)
				opened = append(opened, dw)
				continue
			}
			f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return fail(fmt.Errorf("open log file %s: %w", p, err))
			}
			writers = append(writers, zapcore.AddSync(f))
			opened = append(opened, f)
		}
	}
	if len(writers) == 1 {
		return writers[0], opened, nil
	}
	return zap.CombineWriteSyncers(writers...), opened, nil
}

func closeAll(closers []io.Closer) {
	for _, c := range closers {
		_ = c.Close()
	}
}
