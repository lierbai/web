package web

import (
	"net/http"
	"path"
	"regexp"
	"strings"
)

// IBoarder 定义所有路由器句柄接口,包括单路由器和跳板.
type IBoarder interface {
	IRoutes
	Board(string, ...HandlerFunc) *Boarder
}

// IRoutes 定义所有路由器句柄接口.
type IRoutes interface {
	Use(...HandlerFunc) IRoutes

	Handle(string, string, ...HandlerFunc) IRoutes
	Any(string, ...HandlerFunc) IRoutes
	GET(string, ...HandlerFunc) IRoutes
	POST(string, ...HandlerFunc) IRoutes
	DELETE(string, ...HandlerFunc) IRoutes
	PATCH(string, ...HandlerFunc) IRoutes
	PUT(string, ...HandlerFunc) IRoutes
	OPTIONS(string, ...HandlerFunc) IRoutes
	HEAD(string, ...HandlerFunc) IRoutes

	StaticFile(string, string) IRoutes
	Static(string, string) IRoutes
	StaticFS(string, http.FileSystem) IRoutes
}

// Boarder 在内部用于配置路由器,Boarder前缀和handlers数组(中间件)相关联.
type Boarder struct {
	Handlers HandlersChain
	basePath string
	centre   *Centre
	root     bool
}

var _ IBoarder = &Boarder{}

// Use 给跳板添加中间件.
func (boarder *Boarder) Use(middleware ...HandlerFunc) IRoutes {
	boarder.Handlers = append(boarder.Handlers, middleware...)
	return boarder.returnObj()
}

// Board 创建新的跳板. 您应该添加具有公共中间件或相同路径前缀的所有路由.
func (boarder *Boarder) Board(relativePath string, handlers ...HandlerFunc) *Boarder {
	return &Boarder{
		Handlers: boarder.combineHandlers(handlers),
		basePath: boarder.calculateAbsolutePath(relativePath),
		centre:   boarder.centre,
	}
}

// BasePath 返回跳板的基础路径(相同前缀).
func (boarder *Boarder) BasePath() string {
	return boarder.basePath
}

func (boarder *Boarder) handle(httpMethod, relativePath string, handlers HandlersChain) IRoutes {
	absolutePath := boarder.calculateAbsolutePath(relativePath)
	handlers = boarder.combineHandlers(handlers)
	boarder.centre.addRoute(httpMethod, absolutePath, handlers)
	return boarder.returnObj()
}

// Handle 使用给定的路径和方法注册新的handle和中间件.(批量加载)
// 最后handle才是真正的处理程序,其他应该是公共中间件.允许使用不常用|非标准化|自定义的方法(如,与代理的内部通信)
func (boarder *Boarder) Handle(httpMethod, relativePath string, handlers ...HandlerFunc) IRoutes {
	if matches, err := regexp.MatchString("^[A-Z]+$", httpMethod); !matches || err != nil {
		panic("http method " + httpMethod + " is not valid")
	}
	return boarder.handle(httpMethod, relativePath, handlers)
}

// POST router.Handle("POST", path, handle)的语法糖.
func (boarder *Boarder) POST(relativePath string, handlers ...HandlerFunc) IRoutes {
	return boarder.handle(http.MethodPost, relativePath, handlers)
}

// GET 是router.Handle("GET", path, handle)的语法糖.
func (boarder *Boarder) GET(relativePath string, handlers ...HandlerFunc) IRoutes {
	return boarder.handle(http.MethodGet, relativePath, handlers)
}

// DELETE 是router.Handle("DELETE", path, handle)的语法糖.
func (boarder *Boarder) DELETE(relativePath string, handlers ...HandlerFunc) IRoutes {
	return boarder.handle(http.MethodDelete, relativePath, handlers)
}

// PATCH 是router.Handle("PATCH", path, handle)的语法糖.
func (boarder *Boarder) PATCH(relativePath string, handlers ...HandlerFunc) IRoutes {
	return boarder.handle(http.MethodPatch, relativePath, handlers)
}

// PUT 是router.Handle("PUT", path, handle)的语法糖.
func (boarder *Boarder) PUT(relativePath string, handlers ...HandlerFunc) IRoutes {
	return boarder.handle(http.MethodPut, relativePath, handlers)
}

// OPTIONS 是router.Handle("OPTIONS", path, handle)的语法糖.
func (boarder *Boarder) OPTIONS(relativePath string, handlers ...HandlerFunc) IRoutes {
	return boarder.handle(http.MethodOptions, relativePath, handlers)
}

// HEAD 是router.Handle("HEAD", path, handle)的语法糖.
func (boarder *Boarder) HEAD(relativePath string, handlers ...HandlerFunc) IRoutes {
	return boarder.handle(http.MethodHead, relativePath, handlers)
}

// Any 注册与所有HTTP方法匹配的路由.
// GET, POST, PUT, PATCH, HEAD, OPTIONS, DELETE, CONNECT, TRACE.
func (boarder *Boarder) Any(relativePath string, handlers ...HandlerFunc) IRoutes {
	boarder.handle(http.MethodGet, relativePath, handlers)
	boarder.handle(http.MethodPost, relativePath, handlers)
	boarder.handle(http.MethodPut, relativePath, handlers)
	boarder.handle(http.MethodPatch, relativePath, handlers)
	boarder.handle(http.MethodHead, relativePath, handlers)
	boarder.handle(http.MethodOptions, relativePath, handlers)
	boarder.handle(http.MethodDelete, relativePath, handlers)
	boarder.handle(http.MethodConnect, relativePath, handlers)
	boarder.handle(http.MethodTrace, relativePath, handlers)
	return boarder.returnObj()
}

// StaticFile 静态文件路由注册(单).
// router.StaticFile("favicon.ico", "./resources/favicon.ico")
func (boarder *Boarder) StaticFile(relativePath, filepath string) IRoutes {
	if strings.Contains(relativePath, ":") || strings.Contains(relativePath, "*") {
		panic("URL parameters can not be used when serving a static file")
	}
	handler := func(c *Context) {
		c.File(filepath)
	}
	boarder.GET(relativePath, handler)
	boarder.HEAD(relativePath, handler)
	return boarder.returnObj()
}

// Static 静态文件夹路由注册.
// 内部的 http.FileServer 被使用,因此 http.NotFound 不是用来替代路由的 NotFound handler.
func (boarder *Boarder) Static(relativePath, root string) IRoutes {
	return boarder.StaticFS(relativePath, Dir(root, false))
}

// StaticFS 同Static(),但它有自定义的http.FileSystem替代.
func (boarder *Boarder) StaticFS(relativePath string, fs http.FileSystem) IRoutes {
	if strings.Contains(relativePath, ":") || strings.Contains(relativePath, "*") {
		panic("URL parameters can not be used when serving a static folder")
	}
	handler := boarder.createStaticHandler(relativePath, fs)
	urlPattern := path.Join(relativePath, "/*filepath")

	// Register GET and HEAD handlers
	boarder.GET(urlPattern, handler)
	boarder.HEAD(urlPattern, handler)
	return boarder.returnObj()
}

func (boarder *Boarder) createStaticHandler(relativePath string, fs http.FileSystem) HandlerFunc {
	absolutePath := boarder.calculateAbsolutePath(relativePath)
	fileServer := http.StripPrefix(absolutePath, http.FileServer(fs))

	return func(c *Context) {
		if _, nolisting := fs.(*onlyfilesFS); nolisting {
			c.Writer.WriteHeader(http.StatusNotFound)
		}

		file := c.Param("filepath")
		// Check if file exists and/or if we have permission to access it
		f, err := fs.Open(file)
		if err != nil {
			c.Writer.WriteHeader(http.StatusNotFound)
			c.handlers = boarder.centre.noRoute
			// Reset index
			c.index = -1
			return
		}
		f.Close()

		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func (boarder *Boarder) combineHandlers(handlers HandlersChain) HandlersChain {
	finalSize := len(boarder.Handlers) + len(handlers)
	if finalSize >= int(abortIndex) {
		panic("too many handlers")
	}
	mergedHandlers := make(HandlersChain, finalSize)
	copy(mergedHandlers, boarder.Handlers)
	copy(mergedHandlers[len(boarder.Handlers):], handlers)
	return mergedHandlers
}

func (boarder *Boarder) calculateAbsolutePath(relativePath string) string {
	return joinPaths(boarder.basePath, relativePath)
}

func (boarder *Boarder) returnObj() IRoutes {
	if boarder.root {
		return boarder.centre
	}
	return boarder
}
