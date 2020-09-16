package render

import (
	"fmt"
	"io"
	"net/http"
)

// String 包含给定的接口对象切片及其格式
type String struct {
	Format string
	Data   []interface{}
}

var plainContentType = []string{"text/plain; charset=utf-8"}

// Render (String) 写入数据.
func (r String) Render(w http.ResponseWriter) error {
	writeContentType(w, plainContentType)
	var err error
	if len(r.Data) > 0 {
		_, err = fmt.Fprintf(w, r.Format, r.Data...)
		return err
	}
	_, err = io.WriteString(w, r.Format)
	return err
}

// WriteContentType (String) 写入 Plain ContentType.
func (r String) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, plainContentType)
}
