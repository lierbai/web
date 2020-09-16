package web

import (
	"io"
	"os"

	"github.com/lierbai/web/binding"
)

// EnvWebMode 表示web的环境名称.
const EnvWebMode = "WEB_MODE"

const (
	// DebugMode 表示模式是debug.
	DebugMode = "debug"
	// ReleaseMode 表示模式是release.
	ReleaseMode = "release"
	// TestMode 表示模式是test.
	TestMode = "test"
)
const (
	debugCode = iota
	releaseCode
	testCode
)

// DefaultWriter 默认io.Writer用为debug输出或者中间件输出像Logger()和 Recovery().
// Logger 和 Recovery 都有自定义的io.Writer输出.
var DefaultWriter io.Writer = os.Stdout

// DefaultErrorWriter 用来调试错误的默认io.Writer
var DefaultErrorWriter io.Writer = os.Stderr

var webMode = debugCode
var modeName = DebugMode

func init() {
	mode := os.Getenv(EnvWebMode)
	SetMode(mode)
}

// SetMode 按输入的字符串设置当前的模式(DebugMode,ReleaseMode,TestMode).
func SetMode(value string) {
	switch value {
	case DebugMode, "":
		webMode = debugCode
	case ReleaseMode:
		webMode = releaseCode
	case TestMode:
		webMode = testCode
	default:
		panic("web mode unknown: " + value)
	}
	if value == "" {
		value = DebugMode
	}
	modeName = value
}

// DisableBindValidation 关闭默认验证器.
func DisableBindValidation() {
	binding.Validator = nil
}

// EnableJsonDecoderUseNumber 设置为true,用于调用JSON解码器实例上的UseNumber方法.
func EnableJsonDecoderUseNumber() {
	binding.EnableDecoderUseNumber = true
}

// EnableJsonDecoderDisallowUnknownFields 设置为true.用于调用JSON解码器实例上的DisallowUnknownFields方法
func EnableJsonDecoderDisallowUnknownFields() {
	binding.EnableDecoderDisallowUnknownFields = true
}

// Mode 返回当前的模式名称.
func Mode() string {
	return modeName
}
