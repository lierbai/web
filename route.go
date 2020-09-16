package web

// HandlerFunc 定义中间件的handler为返回值.
type HandlerFunc func(*Context)

// HandlersChain 定义 HandlerFunc 数组.
type HandlersChain []HandlerFunc

// Last 返回Chain里的最后一个Handler. 即. 最后一个Handler才是路由的主Handler.
func (c HandlersChain) Last() HandlerFunc {
	if length := len(c); length > 0 {
		return c[length-1]
	}
	return nil
}

// Route 表示包含方法和路径及其处理程序的请求路由的规范.
type Route struct {
	Method      string
	Path        string
	Handler     string
	HandlerFunc HandlerFunc
}

// Routes defines a RouteInfo array.
type Routes []Route
