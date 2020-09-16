package web

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/mattn/go-isatty"
)

type consoleColorModeValue int

const (
	autoColor consoleColorModeValue = iota
	disableColor
	forceColor
)

const (
	green   = "\033[97;42m"
	white   = "\033[90;47m"
	yellow  = "\033[90;43m"
	red     = "\033[97;41m"
	blue    = "\033[97;44m"
	magenta = "\033[97;45m"
	cyan    = "\033[97;46m"
	reset   = "\033[0m"
)

var consoleColorMode = autoColor

// LoggerConfig 定义日志中间件配置.
type LoggerConfig struct {
	Formatter LogFormatter // 可选格式器.有默认值
	Output    io.Writer    // 可选写入器.有默认值
	SkipPaths []string     // 可选.跳过写入的url路径(数组)
}

// LogFormatter 给定格式器函数的签名传递给LoggerWithFormatter.
type LogFormatter func(params LogFormatterParams) string

// LogFormatterParams 格式器在进行日志记录时需要的参数.
type LogFormatterParams struct {
	Request      *http.Request          // web 请求内容
	TimeStamp    time.Time              // 服务器返回响应后的时间
	StatusCode   int                    // HTTP响应状态码
	Latency      time.Duration          // 处理请求耗时
	ClientIP     string                 // 等于ClientIP方法
	Method       string                 // 来自客户端请求的方法
	Path         string                 // 来自客户端请求的路径
	ErrorMessage string                 // 处理请求时记录错误信息
	isTerm       bool                   // 输出描述符是否指向终端
	BodySize     int                    // 响应体正文大小
	Keys         map[string]interface{} // 请求的上下文中设置的键
}

// StatusCodeColor 给状态码设置适当颜色.(ANSI,输出日志到终端)
func (p *LogFormatterParams) StatusCodeColor() string {
	code := p.StatusCode
	switch {
	case code >= http.StatusOK && code < http.StatusMultipleChoices:
		return green
	case code >= http.StatusMultipleChoices && code < http.StatusBadRequest:
		return white
	case code >= http.StatusBadRequest && code < http.StatusInternalServerError:
		return yellow
	default:
		return red
	}
}

// MethodColor 给状态码设置适当颜色.(输出日志到终端)
func (p *LogFormatterParams) MethodColor() string {
	method := p.Method
	switch method {
	case http.MethodGet:
		return blue
	case http.MethodPost:
		return cyan
	case http.MethodPut:
		return yellow
	case http.MethodDelete:
		return red
	case http.MethodPatch:
		return green
	case http.MethodHead:
		return magenta
	case http.MethodOptions:
		return white
	default:
		return reset
	}
}

// ResetColor 重置所有转义属性.
func (p *LogFormatterParams) ResetColor() string {
	return reset
}

// IsOutputColor 是否可以将颜色输出到日志.
func (p *LogFormatterParams) IsOutputColor() bool {
	return consoleColorMode == forceColor || (consoleColorMode == autoColor && p.isTerm)
}

// defaultLogFormatter 默认日志格式函数.
var defaultLogFormatter = func(param LogFormatterParams) string {
	var statusColor, methodColor, resetColor string
	if param.IsOutputColor() {
		statusColor = param.StatusCodeColor()
		methodColor = param.MethodColor()
		resetColor = param.ResetColor()
	}
	if param.Latency > time.Minute {
		// Truncate in a golang < 1.8 safe way
		param.Latency = param.Latency - param.Latency%time.Second
	}
	return fmt.Sprintf("%v |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
		param.TimeStamp.Format("2006/01/02 - 15:04:05"),
		statusColor, param.StatusCode, resetColor,
		param.Latency,
		param.ClientIP,
		methodColor, param.Method, resetColor,
		param.Path,
		param.ErrorMessage,
	)
}

// DisableConsoleColor 禁用颜色输出(终端).
func DisableConsoleColor() {
	consoleColorMode = disableColor
}

// ForceConsoleColor 强制颜色输出(终端).
func ForceConsoleColor() {
	consoleColorMode = forceColor
}

// Logger 将logger方法转为可用的中间件(上下文)
func Logger() HandlerFunc {
	return LoggerWithConfig(LoggerConfig{})
}

// LoggerWithFormatter 使用指定的日志格式函数实例化记录器中间件.
func LoggerWithFormatter(f LogFormatter) HandlerFunc {
	return LoggerWithConfig(LoggerConfig{
		Formatter: f,
	})
}

// LoggerWithWriter 使用指定的编写器缓冲区实例化记录器中间件.
func LoggerWithWriter(out io.Writer, notlogged ...string) HandlerFunc {
	return LoggerWithConfig(LoggerConfig{
		Output:    out,
		SkipPaths: notlogged,
	})
}

// LoggerWithConfig 使用config实例化记录器中间件.
func LoggerWithConfig(conf LoggerConfig) HandlerFunc {
	formatter := conf.Formatter
	if formatter == nil {
		formatter = defaultLogFormatter
	}

	out := conf.Output
	if out == nil {
		out = DefaultWriter
	}

	notlogged := conf.SkipPaths

	isTerm := true

	if w, ok := out.(*os.File); !ok || os.Getenv("TERM") == "dumb" ||
		(!isatty.IsTerminal(w.Fd()) && !isatty.IsCygwinTerminal(w.Fd())) {
		isTerm = false
	}

	var skip map[string]struct{}

	if length := len(notlogged); length > 0 {
		skip = make(map[string]struct{}, length)

		for _, path := range notlogged {
			skip[path] = struct{}{}
		}
	}

	return func(c *Context) {
		// 开始时间
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 仅当不跳过路径时才记录
		if _, ok := skip[path]; !ok {
			param := LogFormatterParams{
				Request: c.Request,
				isTerm:  isTerm,
				Keys:    c.Keys,
			}

			// 结束定时器
			param.TimeStamp = time.Now()
			param.Latency = param.TimeStamp.Sub(start)

			param.ClientIP = c.ClientIP()
			param.Method = c.Request.Method
			param.StatusCode = c.Writer.Status()
			param.ErrorMessage = c.Errors.ByType(ErrorTypePrivate).String()

			param.BodySize = c.Writer.Size()

			if raw != "" {
				path = path + "?" + raw
			}

			param.Path = path

			fmt.Fprint(out, formatter(param))
		}
	}
}
