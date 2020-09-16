package render

import (
	"fmt"
	"net/http"
)

// Redirect 包含http请求引用并重定向状态代码和重定向地址.
type Redirect struct {
	Code     int
	Request  *http.Request
	Location string
}

// Render (重定向) 将http请求重定向到新地址并写入重定向响应.
func (r Redirect) Render(w http.ResponseWriter) error {
	if (r.Code < http.StatusMultipleChoices || r.Code > http.StatusPermanentRedirect) && r.Code != http.StatusCreated {
		panic(fmt.Sprintf("无法使用的状态代码 %d", r.Code))
	}
	http.Redirect(w, r.Request, r.Location, r.Code)
	return nil
}

// WriteContentType (重定向) 不用写 ContentType.
func (r Redirect) WriteContentType(http.ResponseWriter) {}
