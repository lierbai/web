package render

import "net/http"

// Data 包含ContentType和bytes数据
type Data struct {
	ContentType string
	Data        []byte
}

// Render (Data) 写入数据和自定义ContentType.
func (r Data) Render(w http.ResponseWriter) (err error) {
	r.WriteContentType(w)
	_, err = w.Write(r.Data)
	return
}

// WriteContentType (Data) 写入自定义ContentType.
func (r Data) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, []string{r.ContentType})
}
