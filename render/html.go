package render

import (
	"html/template"
	"net/http"
)

// Delims HTML模板内容渲染分割符
type Delims struct {
	Left  string //左分割符,默认{{
	Right string //右分割符,默认}}
}

// HTMLRender 接口
type HTMLRender interface {
	Instance(string, interface{}) Render
}

// HTMLProduction 模板引用和分隔符对象
type HTMLProduction struct {
	Template *template.Template
	Delims   Delims
}

// HTMLDebug Debug模式额外包含函数和文件列表.便于模板修改(无需重启)
type HTMLDebug struct {
	Files   []string
	Glob    string
	Delims  Delims
	FuncMap template.FuncMap
}

// HTML 模板引用和接口对象名称及数据
type HTML struct {
	Template *template.Template
	Name     string
	Data     interface{}
}

var htmlContentType = []string{"text/html; charset=utf-8"}

// Instance 实例化render接口(HTMLProduction)
func (r HTMLProduction) Instance(name string, data interface{}) Render {
	return HTML{
		Template: r.Template,
		Name:     name,
		Data:     data,
	}
}

func (r HTMLDebug) loadTemplate() *template.Template {
	if r.FuncMap == nil {
		r.FuncMap = template.FuncMap{}
	}
	if len(r.Files) > 0 {
		return template.Must(template.New("").Delims(r.Delims.Left, r.Delims.Right).Funcs(r.FuncMap).ParseFiles(r.Files...))
	}
	if r.Glob != "" {
		return template.Must(template.New("").Delims(r.Delims.Left, r.Delims.Right).Funcs(r.FuncMap).ParseGlob(r.Glob))
	}
	panic("这个HTML调试渲染创建时,没有 文件或公共模版 输入")
}

// Instance 实例化render接口(HTMLDebug)
func (r HTMLDebug) Instance(name string, data interface{}) Render {
	return HTML{
		Template: r.loadTemplate(),
		Name:     name,
		Data:     data,
	}
}

// Render (HTML) 执行模板并将自定义ContentType写入响应体
func (r HTML) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)

	if r.Name == "" {
		return r.Template.Execute(w, r.Data)
	}
	return r.Template.ExecuteTemplate(w, r.Name, r.Data)
}

// WriteContentType 写入 HTML ContentType.
func (r HTML) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, htmlContentType)
}
