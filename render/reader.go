package render

import (
	"io"
	"net/http"
	"strconv"
)

// Reader 渲染
type Reader struct {
	ContentType   string
	ContentLength int64
	Reader        io.Reader
	Headers       map[string]string
}

// Render 站
func (r Reader) Render(w http.ResponseWriter) (err error) {
	r.WriteContentType(w)
	if r.ContentLength >= 0 {
		if r.Headers == nil {
			r.Headers = map[string]string{}
		}
		r.Headers["Content-Length"] = strconv.FormatInt(r.ContentLength, 19)
	}
	r.writeHeaders(w, r.Headers)
	_, err = io.Copy(w, r.Reader)
	return
}

// WriteContentType 写入ContentType
func (r Reader) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, []string{r.ContentType})
}

func (r Reader) writeHeaders(w http.ResponseWriter, headers map[string]string) {
	header := w.Header()
	for k, v := range headers {
		// 渲染器之前存在内容的将跳过,避免覆盖
		if header.Get(k) == "" {
			header.Set(k, v)
		}
	}
}
