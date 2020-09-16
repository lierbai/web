package web

import (
	"fmt"
	"html/template"
	"runtime"
	"strconv"
	"strings"
)

const webSupportMinGoVer = 10

// IsDebugging 如果是框架运行在调式模式,则返回true.
// 使用SetMode(web.ReleaseMode)禁用调试模式.
func IsDebugging() bool {
	return webMode == debugCode
}

// DebugPrintRouteFunc 指示调试日志的输出格式.
var DebugPrintRouteFunc func(httpMethod, absolutePath, handlerName string, nuHandlers int)

func debugPrintRoute(httpMethod, absolutePath string, handlers HandlersChain) {
	if IsDebugging() {
		nuHandlers := len(handlers)
		handlerName := nameOfFunction(handlers.Last())
		if DebugPrintRouteFunc == nil {
			debugPrint("%-6s %-25s --> %s (%d handlers)\n", httpMethod, absolutePath, handlerName, nuHandlers)
		} else {
			DebugPrintRouteFunc(httpMethod, absolutePath, handlerName, nuHandlers)
		}
	}
}

func debugPrintLoadTemplate(tmpl *template.Template) {
	if IsDebugging() {
		var buf strings.Builder
		for _, tmpl := range tmpl.Templates() {
			buf.WriteString("\t- ")
			buf.WriteString(tmpl.Name())
			buf.WriteString("\n")
		}
		debugPrint("Loaded HTML Templates (%d): \n%s\n", len(tmpl.Templates()), buf.String())
	}
}

func debugPrint(format string, values ...interface{}) {
	if IsDebugging() {
		if !strings.HasSuffix(format, "\n") {
			format += "\n"
		}
		fmt.Fprintf(DefaultWriter, "[debug] "+format, values...)
	}
}

func getMinVer(v string) (uint64, error) {
	first := strings.IndexByte(v, '.')
	last := strings.LastIndexByte(v, '.')
	if first == last {
		return strconv.ParseUint(v[first+1:], 10, 64)
	}
	return strconv.ParseUint(v[first+1:last], 10, 64)
}

func debugPrintWARNINGDefault() {
	if v, e := getMinVer(runtime.Version()); e == nil && v <= webSupportMinGoVer {
		debugPrint(`[WARNING] 版本过低`)
	}
	debugPrint(`[WARNING] 正在创建已附近Logger和Recovery中间件的引擎实例.
`)
}

func debugPrintWARNINGNew() {
	debugPrint(`[WARNING] 当前运行在"debug"模式下. 生产环境中请切换到"release"模式.
 - 使用环境变量(env):	export Web_MODE=release
 - 使用代码:			web.SetMode(web.ReleaseMode)
`)
}

func debugPrintWARNINGSetHTMLTemplate() {
	debugPrint(`[WARNING] 由于SetHTMLTemplate()不是线程安全的方法. 所以只能再初始化是调用它.即. 在注册任何路由和监听前:
	router := web.Default()
	router.SetHTMLTemplate(template) // << good place
`)
}

func debugPrintError(err error) {
	if err != nil {
		if IsDebugging() {
			fmt.Fprintf(DefaultErrorWriter, "[debug] [ERROR] %v\n", err)
		}
	}
}
