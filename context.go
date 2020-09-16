package web

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lierbai/web/binding"
	"github.com/lierbai/web/internal/sse"
	"github.com/lierbai/web/render"
)

// Content-Type MIME 常见的数据格式.
const (
	MIMEHTML              = binding.MIMEHTML
	MIMEJSON              = binding.MIMEJSON
	MIMEPlain             = binding.MIMEPlain
	MIMEPOSTForm          = binding.MIMEPOSTForm
	MIMEMultipartPOSTForm = binding.MIMEMultipartPOSTForm
	MIMEXML               = binding.MIMEXML
	MIMEXML2              = binding.MIMEXML2
	BodyBytesKey          = "_lierbai/web/bodybyteskey"
)
const abortIndex int8 = math.MaxInt8 / 2

// Context 上下文管理,web核心,它可以在中间件间传递变量.
type Context struct {
	handlers   HandlersChain          // Handlers 链,方便进行链式处理
	writermem  responseWriter         // 包含size,status和ResponseWriter的结构体
	Writer     ResponseWriter         // ResonseWriter接口
	Request    *http.Request          // 肯定要存储来自客户端的原始请求
	Params     Params                 // URL路径参数,eg:/:name/
	index      int8                   // index
	fullPath   string                 // 全路径地址
	centre     *Centre                // web中枢结构体指针
	mu         sync.RWMutex           // 互斥私有键map
	Keys       map[string]interface{} // 专门用于上下文的键/值对
	Errors     errorMsgs              // Errors 错误列表.
	Accepted   []string               // 定义了允许的格式被用于内容协商(content)
	queryCache url.Values             // 缓存参数查询结果(c.Request.URL.Query())
	formCache  url.Values             // 缓存PostForm包含的表单数据(来自POST,PATCH,PUT)
	sameSite   http.SameSite          // Cookie 限制
}

func (c *Context) reset() {
	c.Writer = &c.writermem
	c.Params = c.Params[0:0]
	c.handlers = nil
	c.index = -1
	c.fullPath = ""
	c.Keys = nil
	c.Errors = c.Errors[0:0]
	c.Accepted = nil
	c.queryCache = nil
	c.formCache = nil
}

// Copy 复制可在请求范围外安全使用的副本.必须将context传递给goroutine时必须使用该方法.
func (c *Context) Copy() *Context {
	cp := Context{
		writermem: c.writermem,
		Request:   c.Request,
		Params:    c.Params,
		centre:    c.centre,
	}
	cp.writermem.ResponseWriter = nil
	cp.Writer = &cp.writermem
	cp.index = abortIndex
	cp.handlers = nil
	cp.Keys = map[string]interface{}{}
	for k, v := range c.Keys {
		cp.Keys[k] = v
	}
	paramCopy := make([]Param, len(cp.Params))
	copy(paramCopy, cp.Params)
	cp.Params = paramCopy
	return &cp
}

// HandlerName 返回主handler的名称.调用hanlerschain中的last,并返回它的名称
func (c *Context) HandlerName() string {
	return nameOfFunction(c.handlers.Last())
}

// HandlerNames 降序返回已注册handlers的列表,遵循HandlerName()的语义
func (c *Context) HandlerNames() []string {
	hn := make([]string, 0, len(c.handlers))
	for _, val := range c.handlers {
		hn = append(hn, nameOfFunction(val))
	}
	return hn
}

// Handler 返回主handler.
func (c *Context) Handler() HandlerFunc {
	return c.handlers.Last()
}

// FullPath 返回所匹配路由的完整路径.未匹配返回''.
//     router.GET("/user/:id", func(c *gin.Context) {
//         c.FullPath() == "/user/:id" // true
//     })
func (c *Context) FullPath() string {
	return c.fullPath
}

/**  流程控制相关函数  **/

// Next 在中间件内被调用.按index来按序调用链内的handlers
func (c *Context) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index](c)
		c.index++
	}
}

// IsAborted 当流程被中止,返回true.
func (c *Context) IsAborted() bool {
	return c.index >= abortIndex
}

// Abort 使链不会执行下一个,但仍会执行调用本方法的handler
func (c *Context) Abort() {
	c.index = abortIndex
}

// AbortWithStatus 调用Abort()方法,并写入响应体状态码.
// 例如,身份验证失败返回401
func (c *Context) AbortWithStatus(code int) {
	c.Status(code)
	c.Writer.WriteHeaderNow()
	c.Abort()
}

// AbortWithStatusJSON 调用Abort()方法,并调用JSON方法.
func (c *Context) AbortWithStatusJSON(code int, jsonObj interface{}) {
	c.Abort()
	c.JSON(code, jsonObj)
}

// AbortWithError 调用AbortWithStatus()和Error()方法.
func (c *Context) AbortWithError(code int, err error) *Error {
	c.AbortWithStatus(code)
	return c.Error(err)
}

/**    错误管理    **/
// Error 附加错误到当前Context,并推送到错误列表.
// 建议在请求解析过程中为每个错误都调用该方法.
// 可以用一个中间件收集所有错误,并推送到数据库中\打印日志\附加到HTTP响应体中.
// 如果错误为空将panic.
func (c *Context) Error(err error) *Error {
	if err == nil {
		panic("err is nil")
	}

	parsedError, ok := err.(*Error)
	if !ok {
		parsedError = &Error{
			Err:  err,
			Type: ErrorTypePrivate,
		}
	}

	c.Errors = append(c.Errors, parsedError)
	return parsedError
}

/**    元(描述)数据管理    **/

// Set 专用于在context中存储新键值对.如果没有使用过 c.Keys ,将初始化它.
func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	if c.Keys == nil {
		c.Keys = make(map[string]interface{})
	}

	c.Keys[key] = value
	c.mu.Unlock()
}

// Get 返回给定的值,和true.空返回(nil, false)
func (c *Context) Get(key string) (value interface{}, exists bool) {
	c.mu.RLock()
	value, exists = c.Keys[key]
	c.mu.RUnlock()
	return
}

// MustGet 如果值存在就返回,否则抛出异常
func (c *Context) MustGet(key string) interface{} {
	if value, exists := c.Get(key); exists {
		return value
	}
	panic("Key \"" + key + "\" does not exist")
}

// GetString 返回字符串类型的给定值.
func (c *Context) GetString(key string) (s string) {
	if val, ok := c.Get(key); ok && val != nil {
		s, _ = val.(string)
	}
	return
}

// GetBool 返回boolean类型的给定值.
func (c *Context) GetBool(key string) (b bool) {
	if val, ok := c.Get(key); ok && val != nil {
		b, _ = val.(bool)
	}
	return
}

// GetInt  返回int类型的给定值.
func (c *Context) GetInt(key string) (i int) {
	if val, ok := c.Get(key); ok && val != nil {
		i, _ = val.(int)
	}
	return
}

// GetInt64 返回int64类型的给定值.
func (c *Context) GetInt64(key string) (i64 int64) {
	if val, ok := c.Get(key); ok && val != nil {
		i64, _ = val.(int64)
	}
	return
}

// GetFloat64 返回float64类型的给定值.
func (c *Context) GetFloat64(key string) (f64 float64) {
	if val, ok := c.Get(key); ok && val != nil {
		f64, _ = val.(float64)
	}
	return
}

// GetTime 返回time类型的给定值.
func (c *Context) GetTime(key string) (t time.Time) {
	if val, ok := c.Get(key); ok && val != nil {
		t, _ = val.(time.Time)
	}
	return
}

// GetDuration 返回Duration(持续时间)类型的给定值.
func (c *Context) GetDuration(key string) (d time.Duration) {
	if val, ok := c.Get(key); ok && val != nil {
		d, _ = val.(time.Duration)
	}
	return
}

// GetStringSlice 返回字符串数组类型的给定值.
func (c *Context) GetStringSlice(key string) (ss []string) {
	if val, ok := c.Get(key); ok && val != nil {
		ss, _ = val.([]string)
	}
	return
}

// GetStringMap 返回map[string]interface{}类型的给定值.
func (c *Context) GetStringMap(key string) (sm map[string]interface{}) {
	if val, ok := c.Get(key); ok && val != nil {
		sm, _ = val.(map[string]interface{})
	}
	return
}

// GetStringMapString 返回map[string]string类型的给定值.
func (c *Context) GetStringMapString(key string) (sms map[string]string) {
	if val, ok := c.Get(key); ok && val != nil {
		sms, _ = val.(map[string]string)
	}
	return
}

// GetStringMapStringSlice 返回map[string][]string类型的给定值.
func (c *Context) GetStringMapStringSlice(key string) (smss map[string][]string) {
	if val, ok := c.Get(key); ok && val != nil {
		smss, _ = val.(map[string][]string)
	}
	return
}

/**    数据输入    **/

// Param 根据键返回URL里的指定值."/user/:id",c.Param("id")返回具体值
func (c *Context) Param(key string) string {
	return c.Params.ByName(key)
}

// Query 返回URL中的值."/path?id=1&name=Manu",c.Query("id")=="1234"
// 相当于 `c.Request.URL.Query().Get(key)`
func (c *Context) Query(key string) string {
	value, _ := c.GetQuery(key)
	return value
}

// DefaultQuery 返回URL中的值(如果存在)."/path?id=1&name=Manu"
// 不存在,它将返回指定的defaultValue字符串.
//     c.DefaultQuery("name", "unknown") == "Manu"
//     c.DefaultQuery("id", "none") == "none"
func (c *Context) DefaultQuery(key, defaultValue string) string {
	if value, ok := c.GetQuery(key); ok {
		return value
	}
	return defaultValue
}

// GetQuery 返回URL中的值."/path?lastname="
// 存在(即使值是空的)返回(value, true),否则返回("", false).
//     ("", false) == c.GetQuery("id")
//     ("", true) == c.GetQuery("lastname")
func (c *Context) GetQuery(key string) (string, bool) {
	if values, ok := c.GetQueryArray(key); ok {
		return values[0], ok
	}
	return "", false
}

// QueryArray 返回指定键的[]string.长度取决于包含指定键的参数数量.
func (c *Context) QueryArray(key string) []string {
	values, _ := c.GetQueryArray(key)
	return values
}

func (c *Context) getQueryCache() {
	if c.queryCache == nil {
		c.queryCache = c.Request.URL.Query()
	}
}

// GetQueryArray 返回(values, true),不存在则返回([]string{}, false).
func (c *Context) GetQueryArray(key string) ([]string, bool) {
	c.getQueryCache()
	if values, ok := c.queryCache[key]; ok && len(values) > 0 {
		return values, true
	}
	return []string{}, false
}

// QueryMap returns a map for a given query key.
func (c *Context) QueryMap(key string) map[string]string {
	dicts, _ := c.GetQueryMap(key)
	return dicts
}

// GetQueryMap 返回指定键映射的map[string]string和bool(存在为true,包含空).
func (c *Context) GetQueryMap(key string) (map[string]string, bool) {
	c.getQueryCache()
	return c.get(c.queryCache, key)
}

// PostForm 返回POSTurlencoded form或multipart form指定键的值,或返回""
func (c *Context) PostForm(key string) string {
	value, _ := c.GetPostForm(key)
	return value
}

// DefaultPostForm 如上,但是为空时返回指定的默认值.
func (c *Context) DefaultPostForm(key, defaultValue string) string {
	if value, ok := c.GetPostForm(key); ok {
		return value
	}
	return defaultValue
}

// GetPostForm 如PostForm(key).从POST urlencoded返回(string, bool)
// 即使不存在也会返回("", false).存在键为空也会返回("", true)
func (c *Context) GetPostForm(key string) (string, bool) {
	if values, ok := c.GetPostFormArray(key); ok {
		return values[0], ok
	}
	return "", false
}

// PostFormArray 根据表单key返回字符串数组.
func (c *Context) PostFormArray(key string) []string {
	values, _ := c.GetPostFormArray(key)
	return values
}

func (c *Context) getFormCache() {
	if c.formCache == nil {
		c.formCache = make(url.Values)
		req := c.Request
		if err := req.ParseMultipartForm(c.centre.MaxMultipartMemory); err != nil {
			if err != http.ErrNotMultipart {
				debugPrint("error on parse multipart form array: %v", err)
			}
		}
		c.formCache = req.PostForm
	}
}

// GetPostFormArray 返回([]string, bool),len>0为真.
func (c *Context) GetPostFormArray(key string) ([]string, bool) {
	c.getFormCache()
	if values := c.formCache[key]; len(values) > 0 {
		return values, true
	}
	return []string{}, false
}

// PostFormMap 按key返回map[string]string.
func (c *Context) PostFormMap(key string) map[string]string {
	dicts, _ := c.GetPostFormMap(key)
	return dicts
}

// GetPostFormMap 返回([]string, bool),len>0为真.
func (c *Context) GetPostFormMap(key string) (map[string]string, bool) {
	c.getFormCache()
	return c.get(c.formCache, key)
}

// get 返回满足条件的(map[string]string, bool).
func (c *Context) get(m map[string][]string, key string) (map[string]string, bool) {
	dicts := make(map[string]string)
	exist := false
	for k, v := range m {
		if i := strings.IndexByte(k, '['); i >= 1 && k[0:i] == key {
			if j := strings.IndexByte(k[i+1:], ']'); j >= 1 {
				exist = true
				dicts[k[i+1:][:j]] = v[0]
			}
		}
	}
	return dicts, exist
}

// FormFile 按键返回第一个文件.
func (c *Context) FormFile(name string) (*multipart.FileHeader, error) {
	if c.Request.MultipartForm == nil {
		if err := c.Request.ParseMultipartForm(c.centre.MaxMultipartMemory); err != nil {
			return nil, err
		}
	}
	f, fh, err := c.Request.FormFile(name)
	if err != nil {
		return nil, err
	}
	f.Close()
	return fh, err
}

// MultipartForm 返回解析的multipart form(包含文件上传).
func (c *Context) MultipartForm() (*multipart.Form, error) {
	err := c.Request.ParseMultipartForm(c.centre.MaxMultipartMemory)
	return c.Request.MultipartForm, err
}

// SaveUploadedFile 上传表单文件到指定的dst.
func (c *Context) SaveUploadedFile(file *multipart.FileHeader, dst string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

// Bind 根据不同的Content-Type自动选择绑定,没有可以兼容的绑定将返回错误.
// 如果输入无效将响应状态码400并设置Content-Type header 为"text/plain"
func (c *Context) Bind(obj interface{}) error {
	b := binding.Default(c.Request.Method, c.ContentType())
	return c.MustBindWith(obj, b)
}

// BindJSON c.MustBindWith(obj, binding.JSON)的语法糖.
func (c *Context) BindJSON(obj interface{}) error {
	return c.MustBindWith(obj, binding.JSON)
}

// BindXML c.MustBindWith(obj, binding.BindXML)的语法糖.
func (c *Context) BindXML(obj interface{}) error {
	return c.MustBindWith(obj, binding.XML)
}

// BindQuery c.MustBindWith(obj, binding.Query)的语法糖.
func (c *Context) BindQuery(obj interface{}) error {
	return c.MustBindWith(obj, binding.Query)
}

// BindHeader c.MustBindWith(obj, binding.Header)的语法糖.
func (c *Context) BindHeader(obj interface{}) error {
	return c.MustBindWith(obj, binding.Header)
}

// BindUri 使用 binding.Uri绑定传递的struct指针.错误返回http400.
func (c *Context) BindUri(obj interface{}) error {
	if err := c.ShouldBindUri(obj); err != nil {
		c.AbortWithError(http.StatusBadRequest, err).SetType(ErrorTypeBind) // nolint: errcheck
		return err
	}
	return nil
}

// MustBindWith 使用binding engine绑定传递的struct指针.错误返回http400.
func (c *Context) MustBindWith(obj interface{}, b binding.Binding) error {
	if err := c.ShouldBindWith(obj, b); err != nil {
		c.AbortWithError(http.StatusBadRequest, err).SetType(ErrorTypeBind) // nolint: errcheck
		return err
	}
	return nil
}

// ShouldBind  根据不同的Content-Type自动选择绑定,没有可以兼容的绑定将中止.
func (c *Context) ShouldBind(obj interface{}) error {
	b := binding.Default(c.Request.Method, c.ContentType())
	return c.ShouldBindWith(obj, b)
}

// ShouldBindJSON c.ShouldBindWith(obj, binding.JSON)的语法糖.
func (c *Context) ShouldBindJSON(obj interface{}) error {
	return c.ShouldBindWith(obj, binding.JSON)
}

// ShouldBindXML c.ShouldBindWith(obj, binding.XML)的语法糖.
func (c *Context) ShouldBindXML(obj interface{}) error {
	return c.ShouldBindWith(obj, binding.XML)
}

// ShouldBindQuery c.ShouldBindWith(obj, binding.Query)的语法糖.
func (c *Context) ShouldBindQuery(obj interface{}) error {
	return c.ShouldBindWith(obj, binding.Query)
}

// ShouldBindHeader c.ShouldBindWith(obj, binding.Header)的语法糖.
func (c *Context) ShouldBindHeader(obj interface{}) error {
	return c.ShouldBindWith(obj, binding.Header)
}

// ShouldBindUri  使用 binding.Uri 绑定传递的struct指针.
func (c *Context) ShouldBindUri(obj interface{}) error {
	m := make(map[string][]string)
	for _, v := range c.Params {
		m[v.Key] = []string{v.Value}
	}
	return binding.Uri.BindUri(m, obj)
}

// ShouldBindWith 使用 binding engine 绑定传递的struct指针 .
func (c *Context) ShouldBindWith(obj interface{}, b binding.Binding) error {
	return b.Bind(c.Request, obj)
}

// ShouldBindBodyWith 将请求体存储在context,并可以在再次调用时使用.
// 值得注意的是,此函数在绑定前读取,如果只需读取一次,用它可以获得更好的性能体验.
func (c *Context) ShouldBindBodyWith(obj interface{}, bb binding.BindingBody) (err error) {
	var body []byte
	if cb, ok := c.Get(BodyBytesKey); ok {
		if cbb, ok := cb.([]byte); ok {
			body = cbb
		}
	}
	if body == nil {
		body, err = ioutil.ReadAll(c.Request.Body)
		if err != nil {
			return err
		}
		c.Set(BodyBytesKey, body)
	}
	return bb.BindBody(body, obj)
}

// ClientIP 实现一个尽力返回真实客户端IP的算法, 分析X-Real-IP 和 X-Forwarded-For以便正确处理反向代理,如: nginx|haproxy.
// Use X-Forwarded-For before X-Real-Ip as nginx uses X-Real-Ip with the proxy's IP.
func (c *Context) ClientIP() string {
	if c.centre.ForwardedByClientIP {
		clientIP := c.requestHeader("X-Forwarded-For")
		clientIP = strings.TrimSpace(strings.Split(clientIP, ",")[0])
		if clientIP == "" {
			clientIP = strings.TrimSpace(c.requestHeader("X-Real-Ip"))
		}
		if clientIP != "" {
			return clientIP
		}
	}

	if c.centre.AppCentre {
		if addr := c.requestHeader("X-Appengine-Remote-Addr"); addr != "" {
			return addr
		}
	}

	if ip, _, err := net.SplitHostPort(strings.TrimSpace(c.Request.RemoteAddr)); err == nil {
		return ip
	}

	return ""
}

// ContentType 返回请求header的Content-Type.
func (c *Context) ContentType() string {
	return filterFlags(c.requestHeader("Content-Type"))
}

// IsWebsocket 如果请求headers显示客户端正常启动websocket握手,返回true
func (c *Context) IsWebsocket() bool {
	if strings.Contains(strings.ToLower(c.requestHeader("Connection")), "upgrade") &&
		strings.EqualFold(c.requestHeader("Upgrade"), "websocket") {
		return true
	}
	return false
}

func (c *Context) requestHeader(key string) string {
	return c.Request.Header.Get(key)
}

/**    响应体渲染    **/

// bodyAllowedForStatus 是http.bodyAllowedForStatus的非输出函数.
func bodyAllowedForStatus(status int) bool {
	switch {
	case status >= 100 && status <= 199:
		return false
	case status == http.StatusNoContent:
		return false
	case status == http.StatusNotModified:
		return false
	}
	return true
}

// Status 设置HTTP响应代码.
func (c *Context) Status(code int) {
	c.Writer.WriteHeader(code)
}

// Header 是c.Writer.Header().Set(key, value)的语法糖.写入header.
// 如果值为空,将删除该header,即`c.Writer.Header().Del(key)`
func (c *Context) Header(key, value string) {
	if value == "" {
		c.Writer.Header().Del(key)
		return
	}
	c.Writer.Header().Set(key, value)
}

// GetHeader 从请求的headers里返回值.
func (c *Context) GetHeader(key string) string {
	return c.requestHeader(key)
}

// GetRawData 返回流数据.
func (c *Context) GetRawData() ([]byte, error) {
	return ioutil.ReadAll(c.Request.Body)
}

// SetSameSite 同 cookie
func (c *Context) SetSameSite(samesite http.SameSite) {
	c.sameSite = samesite
}

// SetCookie 将Set-Cookie header 添加到ResponseWriter的headers中.
// 提供的cookie必须是有效的(无效可能会被静默丢弃).
func (c *Context) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	if path == "" {
		path = "/"
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    url.QueryEscape(value),
		MaxAge:   maxAge,
		Path:     path,
		Domain:   domain,
		SameSite: c.sameSite,
		Secure:   secure,
		HttpOnly: httpOnly,
	})
}

// Cookie 返回请求中名为name的cookies(未转义的).
// 如果有多个同名 cookies 则只返回一个.
func (c *Context) Cookie(name string) (string, error) {
	cookie, err := c.Request.Cookie(name)
	if err != nil {
		return "", err
	}
	val, _ := url.QueryUnescape(cookie.Value)
	return val, nil
}

// Render 写入响应体headers和调用 render.Render 渲染数据.
func (c *Context) Render(code int, r render.Render) {
	c.Status(code)

	if !bodyAllowedForStatus(code) {
		r.WriteContentType(c.Writer)
		c.Writer.WriteHeaderNow()
		return
	}

	if err := r.Render(c.Writer); err != nil {
		panic(err)
	}
}

// HTML 渲染模版(指定模板文件,随手设置了响应状态码和Content-Type).
func (c *Context) HTML(code int, name string, obj interface{}) {
	instance := c.centre.HTMLRender.Instance(name, obj)
	c.Render(code, instance)
}

// IndentedJSON 将给定的结构序列化为JSON (缩进+换行)并写入.
// (随手设置了Content-Type).
// 警告: 该方法虽然可读性高,但会比JSON()消耗更多资源,所以建议只在开发中使用.
func (c *Context) IndentedJSON(code int, obj interface{}) {
	c.Render(code, render.JSON{Indented: true, Data: obj})
}

// JSON 将给定的结构序列化为JSON并写入(随手设置了Content-Type).
// 在开发模式中,JSON 呈现为 (缩进+换行)
func (c *Context) JSON(code int, obj interface{}) {
	c.Render(code, render.JSON{Indented: IsDebugging(), Data: obj})
}

// AsciiJSON 将给定的结构序列化为JSON并使用ASCII格式写入.(随手设置了Content-Type).
func (c *Context) AsciiJSON(code int, obj interface{}) {
	c.Render(code, render.JSON{IsAscii: true, Data: obj})
}

// PureJSON 将给定的结构序列化为JSON并写入(不使用unicode替换特殊字符).
func (c *Context) PureJSON(code int, obj interface{}) {
	c.Render(code, render.JSON{IsPrue: true, Data: obj})
}

// XML 将给定的结构序列化为XML并写入response body.(随手设置了Content-Type)
func (c *Context) XML(code int, obj interface{}) {
	c.Render(code, render.XML{Data: obj})
}

// String 将给定字符串写入响应正文.
func (c *Context) String(code int, format string, values ...interface{}) {
	c.Render(code, render.String{Format: format, Data: values})
}

// Redirect 返回location的HTTP重定向.
func (c *Context) Redirect(code int, location string) {
	c.Render(-1, render.Redirect{
		Code:     code,
		Location: location,
		Request:  c.Request,
	})
}

// Data writes 将Data写入到body流并更新响应状态.
func (c *Context) Data(code int, contentType string, data []byte) {
	c.Render(code, render.Data{
		ContentType: contentType,
		Data:        data,
	})
}

// DataFromReader 使用指定的读取器写入body流并更新响应状态.
func (c *Context) DataFromReader(code int, contentLength int64, contentType string, reader io.Reader, extraHeaders map[string]string) {
	c.Render(code, render.Reader{
		Headers:       extraHeaders,
		ContentType:   contentType,
		ContentLength: contentLength,
		Reader:        reader,
	})
}

// File 以有效的方式将指定的文件写入body流.
func (c *Context) File(filepath string) {
	http.ServeFile(c.Writer, c.Request, filepath)
}

// FileFromFS 从 http.FileSystem 将指定的文件写入body流.
func (c *Context) FileFromFS(filepath string, fs http.FileSystem) {
	defer func(old string) {
		c.Request.URL.Path = old
	}(c.Request.URL.Path)

	c.Request.URL.Path = filepath

	http.FileServer(fs).ServeHTTP(c.Writer, c.Request)
}

// FileAttachment 以有效的方式将指定的文件写入body流
// 在客户端，文件通常是用给定的文件名下载的
func (c *Context) FileAttachment(filepath, filename string) {
	c.Writer.Header().Set("content-disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	http.ServeFile(c.Writer, c.Request, filepath)
}

// SSEvent 将服务器发送的事件写入body流.
func (c *Context) SSEvent(name string, message interface{}) {
	c.Render(-1, sse.Event{Event: name, Data: message})
}

// Stream 发送流式响应并返回布尔值，指示"是否在流中间断开客户端连接"
func (c *Context) Stream(step func(w io.Writer) bool) bool {
	w := c.Writer
	clientGone := w.CloseNotify()
	for {
		select {
		case <-clientGone:
			return true
		default:
			keepOpen := step(w)
			w.Flush()
			if !keepOpen {
				return false
			}
		}
	}
}

/**    negotiations内容    **/

// Negotiate 包含所有negotiations数据.
type Negotiate struct {
	Offered  []string
	HTMLName string
	HTMLData interface{}
	JSONData interface{}
	XMLData  interface{}
	Data     interface{}
}

// Negotiate 调用不同的渲染器(可调用的).
func (c *Context) Negotiate(code int, config Negotiate) {
	switch c.NegotiateFormat(config.Offered...) {
	case binding.MIMEJSON:
		data := chooseData(config.JSONData, config.Data)
		c.JSON(code, data)

	case binding.MIMEHTML:
		data := chooseData(config.HTMLData, config.Data)
		c.HTML(code, config.HTMLName, data)

	case binding.MIMEXML:
		data := chooseData(config.XMLData, config.Data)
		c.XML(code, data)

	default:
		c.AbortWithError(http.StatusNotAcceptable, errors.New("不被接受的格式")) // nolint: errcheck
	}
}

// NegotiateFormat 返回可用的Accept格式.
func (c *Context) NegotiateFormat(offered ...string) string {
	assert1(len(offered) > 0, "你至少需要提供一个参数")

	if c.Accepted == nil {
		c.Accepted = parseAccept(c.requestHeader("Accept"))
	}
	if len(c.Accepted) == 0 {
		return offered[0]
	}
	for _, accepted := range c.Accepted {
		for _, offer := range offered {
			// 根据RFC 2616 和 RFC 2396, non-ASCII 不允许被用于headers中,
			// 所以可以在字符串上迭代,不必转为[]rune
			i := 0
			for ; i < len(accepted); i++ {
				if accepted[i] == '*' || offer[i] == '*' {
					return offer
				}
				if accepted[i] != offer[i] {
					break
				}
			}
			if i == len(accepted) {
				return offer
			}
		}
	}
	return ""
}

// SetAccepted 设置 Accept header data.
func (c *Context) SetAccepted(formats ...string) {
	c.Accepted = formats
}

/**    响应体渲染    **/

// Deadline 恒返回 无截止日期 (ok==false),
// 也许你应该想调用 Request.Context().Deadline() ?
func (c *Context) Deadline() (deadline time.Time, ok bool) {
	return
}

// Done  恒返回nil(chan将永远等待)
// 如果你想在连接关闭时中止,应该使用 Request.Context().Done().
func (c *Context) Done() <-chan struct{} {
	return nil
}

// Err 恒返回nil,可以用 Request.Context().Err() 替代.
func (c *Context) Err() error {
	return nil
}

// Value 返回与键关联的值，如果没有值与键关联，则返回nil.
// 使用相同的键连续调用值将返回相同的结果.
func (c *Context) Value(key interface{}) interface{} {
	if key == 0 {
		return c.Request
	}
	if keyAsString, ok := key.(string); ok {
		val, _ := c.Get(keyAsString)
		return val
	}
	return nil
}
