package web

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"sync"
	"text/template"

	"github.com/lierbai/web/internal/bytesconv"
	"github.com/lierbai/web/render"
)

const defaultMultipartMemory = 32 << 20 // 32 MB

var (
	default404Body   = []byte("404 page not found")
	default405Body   = []byte("405 method not allowed")
	defaultAppCentre bool
)

// Centre 中枢
type Centre struct {
	Boarder
	RedirectTrailingSlash  bool              // 反斜杠结尾路径自动重定向
	HandleMethodNotAllowed bool              // 请求体内部转递
	ForwardedByClientIP    bool              // 转发连接IP
	UseRawPath             bool              // url.RawPath查找参数
	UnescapePathValues     bool              // 不转义,使用url.Path
	RemoveExtraSlash       bool              // 是否删除额外的反斜杠
	MaxMultipartMemory     int64             // 表单上传最大限制
	delims                 render.Delims     // 模板参数识别分隔符
	HTMLRender             render.HTMLRender // 返回渲染模板的接口
	FuncMap                template.FuncMap  // 名称到函数的映射
	pool                   sync.Pool         // 线程安全队列
	allNoRoute             HandlersChain     //
	allNoMethod            HandlersChain     //
	noRoute                HandlersChain     //
	noMethod               HandlersChain     //
	trees                  methodTrees       // 路径节点树
}

// New 返回未附加任何中间件的Centre实例
func New() *Centre {
	debugPrintWARNINGNew()
	centre := &Centre{
		Boarder: Boarder{
			Handlers: nil,
			basePath: "/",
			root:     true,
		},
		RedirectTrailingSlash:  true,
		HandleMethodNotAllowed: false,
		ForwardedByClientIP:    true,
		UseRawPath:             false,
		UnescapePathValues:     true,
		RemoveExtraSlash:       false,
		MaxMultipartMemory:     defaultMultipartMemory, // 32 MB
		delims:                 render.Delims{Left: "{{", Right: "}}"},
		FuncMap:                template.FuncMap{},
		trees:                  make(methodTrees, 0, 9),
	}
	centre.Boarder.centre = centre
	centre.pool.New = func() interface{} {
		return centre.allocateContext()
	}
	return centre
}

// Default 返回已附加记录器和恢复中间件的Centre实例
func Default() *Centre {
	debugPrintWARNINGDefault()
	centre := New()
	centre.Use(Logger(), Recovery())
	return centre
}

func (centre *Centre) allocateContext() *Context {
	return &Context{centre: centre}
}

// Delims 设置模板变量的左右分隔符并返回实例
func (centre *Centre) Delims(left, right string) *Centre {
	centre.delims = render.Delims{Left: left, Right: right}
	return centre
}

// LoadHTMLGlob 加载由glob模式标识的HTML文件,并将结果与HTML呈现器关联
func (centre *Centre) LoadHTMLGlob(pattern string) {
	templ := template.Must(template.New("").Delims(centre.delims.Left, centre.delims.Right).Funcs(centre.FuncMap).ParseGlob(pattern))

	if IsDebugging() {
		debugPrintLoadTemplate(templ)
		centre.HTMLRender = render.HTMLDebug{Glob: pattern, Delims: centre.delims, FuncMap: centre.FuncMap}
		return
	}
	centre.SetHTMLTemplate(templ)
}

// LoadHTMLFiles 加载HTML文件片段并将结果与HTML呈现器关联
func (centre *Centre) LoadHTMLFiles(files ...string) {
	if IsDebugging() {
		centre.HTMLRender = render.HTMLDebug{Files: files, Delims: centre.delims, FuncMap: centre.FuncMap}
		return
	}
	templ := template.Must(template.New("").Delims(centre.delims.Left, centre.delims.Right).Funcs(centre.FuncMap).ParseFiles(files...))
	centre.SetHTMLTemplate(templ)

}

// SetHTMLTemplate 将模板与HTML呈现器关联.
func (centre *Centre) SetHTMLTemplate(templ *template.Template) {
	if len(centre.trees) > 0 {
		debugPrintWARNINGSetHTMLTemplate()
	}

	centre.HTMLRender = render.HTMLProduction{Template: templ.Funcs(centre.FuncMap)}
}

// SetFuncMap 设置 FuncMap 的值 template.FuncMap.
func (centre *Centre) SetFuncMap(funcMap template.FuncMap) {
	centre.FuncMap = funcMap
}

// NoRoute 为NoRoute添加handlers. 默认返回404代码.
func (centre *Centre) NoRoute(handlers ...HandlerFunc) {
	centre.noRoute = handlers
	centre.rebuild404Handlers()
}

// NoMethod 为NoMethod添加handlers. 默认返回405代码.
func (centre *Centre) NoMethod(handlers ...HandlerFunc) {
	centre.noMethod = handlers
	centre.rebuild405Handlers()
}

// Use 将全局中间件连接到路由器. ie.通过 Use() 附加的中间件将包含在每个请求的处理程序链中. 甚至是 404, 405, 静态文件...
// 这是日志记录器和错误管理中间件的好位置
func (centre *Centre) Use(middleware ...HandlerFunc) IRoutes {
	centre.Boarder.Use(middleware...)
	centre.rebuild404Handlers()
	centre.rebuild405Handlers()
	return centre
}

func (centre *Centre) rebuild404Handlers() {
	centre.allNoRoute = centre.combineHandlers(centre.noRoute)
}

func (centre *Centre) rebuild405Handlers() {
	centre.allNoMethod = centre.combineHandlers(centre.noMethod)
}

func (centre *Centre) addRoute(method, path string, handlers HandlersChain) {
	assert1(path[0] == '/', "路径必须以'/'开头")
	assert1(method != "", "HTTP method 不能为空")
	assert1(len(handlers) > 0, "必须至少有一个处理程序")

	debugPrintRoute(method, path, handlers)
	root := centre.trees.get(method)
	if root == nil {
		root = new(node)
		root.fullPath = "/"
		centre.trees = append(centre.trees, methodTree{method: method, root: root})
	}
	root.addRoute(path, handlers)
}

// Routes Routes
func (centre *Centre) Routes() (routes Routes) {
	for _, tree := range centre.trees {
		routes = iterate("", tree.method, routes, tree.root)
	}
	return routes
}

func iterate(path, method string, routes Routes, root *node) Routes {
	path += root.path
	if len(root.handlers) > 0 {
		handlerFunc := root.handlers.Last()
		routes = append(routes, Route{
			Method:      method,
			Path:        path,
			Handler:     nameOfFunction(handlerFunc),
			HandlerFunc: handlerFunc,
		})
	}
	for _, child := range root.children {
		routes = iterate(path, method, routes, child)
	}
	return routes
}

// Run 监听并服务请求.
// http.ListenAndServe(addr, router) 的快捷实现
// Note: 除非发生错误,否则此方法将一直阻止调用 goroutine
func (centre *Centre) Run(addr ...string) (err error) {
	defer func() { debugPrintError(err) }()

	address := resolveAddress(addr)
	debugPrint("Listening and serving HTTP on %s\n", address)
	err = http.ListenAndServe(address, centre)
	return
}

// RunTLS 监听并服务HTTPS (secure)请求.
// http.ListenAndServeTLS(addr, certFile, keyFile, router) 的快捷实现
// Note: 除非发生错误,否则此方法将一直阻止调用 goroutine
func (centre *Centre) RunTLS(addr, certFile, keyFile string) (err error) {
	debugPrint("Listening and serving HTTPS on %s\n", addr)
	defer func() { debugPrintError(err) }()

	err = http.ListenAndServeTLS(addr, certFile, keyFile, centre)
	return
}

// RunUnix attaches the router to a http.Server and starts listening and serving HTTP requests
// 通过指定的unix socket (ie. 文件).
// Note: 除非发生错误,否则此方法将一直阻止调用 goroutine
func (centre *Centre) RunUnix(file string) (err error) {
	debugPrint("Listening and serving HTTP on unix:/%s", file)
	defer func() { debugPrintError(err) }()

	listener, err := net.Listen("unix", file)
	if err != nil {
		return
	}
	defer listener.Close()
	defer os.Remove(file)

	err = http.Serve(listener, centre)
	return
}

// RunFd 监听并服务HTTP请求.通过指定的文件描述符.
// Note: 除非发生错误,否则此方法将一直阻止调用 goroutine
func (centre *Centre) RunFd(fd int) (err error) {
	debugPrint("Listening and serving HTTP on fd@%d", fd)
	defer func() { debugPrintError(err) }()

	f := os.NewFile(uintptr(fd), fmt.Sprintf("fd@%d", fd))
	listener, err := net.FileListener(f)
	if err != nil {
		return
	}
	defer listener.Close()
	err = centre.RunListener(listener)
	return
}

// RunListener 监听并服务HTTP请求.通过指定的文件描述符.
// 通过指定的net.Listener
func (centre *Centre) RunListener(listener net.Listener) (err error) {
	debugPrint("Listening and serving HTTP on listener what's bind with address@%s", listener.Addr())
	defer func() { debugPrintError(err) }()
	err = http.Serve(listener, centre)
	return
}

// ServeHTTP 遵从 http.Handler 接口.
func (centre *Centre) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c := centre.pool.Get().(*Context)
	c.writermem.reset(w)
	c.Request = req
	c.reset()

	centre.handleHTTPRequest(c)

	centre.pool.Put(c)
}

// HandleContext 重新输入已重写的上下文.
// 这可以通过将 c.Request.URL.Path 设置为新目标来完成.
// Disclaimer: You can loop yourself to death with this, use wisely.
func (centre *Centre) HandleContext(c *Context) {
	oldIndexValue := c.index
	c.reset()
	centre.handleHTTPRequest(c)

	c.index = oldIndexValue
}

func (centre *Centre) handleHTTPRequest(c *Context) {
	httpMethod := c.Request.Method
	rPath := c.Request.URL.Path
	unescape := false
	if centre.UseRawPath && len(c.Request.URL.RawPath) > 0 {
		rPath = c.Request.URL.RawPath
		unescape = centre.UnescapePathValues
	}

	if centre.RemoveExtraSlash {
		rPath = cleanPath(rPath)
	}

	// 为给定的HTTP方法查找数的根节点
	t := centre.trees
	for i, tl := 0, len(t); i < tl; i++ {
		if t[i].method != httpMethod {
			continue
		}
		root := t[i].root
		// 在树中查找路由
		value := root.getValue(rPath, c.Params, unescape)
		if value.handlers != nil {
			c.handlers = value.handlers
			c.Params = value.params
			c.fullPath = value.fullPath
			c.Next()
			c.writermem.WriteHeaderNow()
			return
		}
		if httpMethod != "CONNECT" && rPath != "/" {
			if value.tsr && centre.RedirectTrailingSlash {
				redirectTrailingSlash(c)
				return
			}
			return
			// if centre.RedirectFixedPath && redirectFixedPath(c, root, centre.RedirectFixedPath) {
			// 	return
			// }
		}
		break
	}

	if centre.HandleMethodNotAllowed {
		for _, tree := range centre.trees {
			if tree.method == httpMethod {
				continue
			}
			if value := tree.root.getValue(rPath, nil, unescape); value.handlers != nil {
				c.handlers = centre.allNoMethod
				serveError(c, http.StatusMethodNotAllowed, default405Body)
				return
			}
		}
	}
	c.handlers = centre.allNoRoute
	serveError(c, http.StatusNotFound, default404Body)
}

var mimePlain = []string{MIMEPlain}

func serveError(c *Context, code int, defaultMessage []byte) {
	c.writermem.status = code
	c.Next()
	if c.writermem.Written() {
		return
	}
	if c.writermem.Status() == code {
		c.writermem.Header()["Content-Type"] = mimePlain
		_, err := c.Writer.Write(defaultMessage)
		if err != nil {
			debugPrint("cannot write message to writer during serve error: %v", err)
		}
		return
	}
	c.writermem.WriteHeaderNow()
}

func redirectTrailingSlash(c *Context) {
	req := c.Request
	p := req.URL.Path
	if prefix := path.Clean(c.Request.Header.Get("X-Forwarded-Prefix")); prefix != "." {
		p = prefix + "/" + req.URL.Path
	}
	req.URL.Path = p + "/"
	if length := len(p); length > 1 && p[length-1] == '/' {
		req.URL.Path = p[:length-1]
	}
	redirectRequest(c)
}

func redirectFixedPath(c *Context, root *node, trailingSlash bool) bool {
	req := c.Request
	rPath := req.URL.Path

	if fixedPath, ok := root.findCaseInsensitivePath(cleanPath(rPath), trailingSlash); ok {
		req.URL.Path = bytesconv.BytesToString(fixedPath)
		redirectRequest(c)
		return true
	}
	return false
}

func redirectRequest(c *Context) {
	req := c.Request
	rPath := req.URL.Path
	rURL := req.URL.String()
	code := http.StatusMovedPermanently // 永久重定向，使用GET方法请求
	if req.Method != http.MethodGet {
		code = http.StatusTemporaryRedirect
	}
	debugPrint("redirecting request %d: %s --> %s", code, rPath, rURL)
	http.Redirect(c.Writer, req, rURL, code)
	c.writermem.WriteHeaderNow()
}
