package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/lierbai/web/internal/bytesconv"
)

// JSON 包含给定的数据接口对象.
type JSON struct {
	Indented bool
	IsAscii  bool
	IsPrue   bool
	Data     interface{}
}

var jsonContentType = []string{"application/json; charset=utf-8"}
var jsonAsciiContentType = []string{"application/json"}

// Render (JSON) 写入数据 和 ContentType
func (r JSON) Render(w http.ResponseWriter) (err error) {
	r.WriteContentType(w)
	if r.IsPrue {
		encoder := json.NewEncoder(w)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(r.Data)
	}
	jsonBytes, err := r.marshal()
	if err != nil {
		return err
	}
	if r.IsAscii {
		jsonBytes = r.write(jsonBytes)
	}
	_, err = w.Write(jsonBytes)
	return err
}

// WriteContentType (JSON) 写入 JSON ContentType.
func (r JSON) WriteContentType(w http.ResponseWriter) {
	// 写入 ContentType
	if !r.IsAscii {
		writeContentType(w, jsonContentType)
	} else {
		writeContentType(w, jsonAsciiContentType)
	}
}

// Marshal 按需求进行转换
func (r JSON) marshal() ([]byte, error) {
	if !r.Indented {
		return json.Marshal(r.Data)
	}
	return json.MarshalIndent(r.Data, "", "    ")
}

// write 执行写入
func (r JSON) write(jsonBytes []byte) []byte {
	var buffer bytes.Buffer
	for _, r := range bytesconv.BytesToString(jsonBytes) {
		cvt := string(r)
		if r >= 128 {
			cvt = fmt.Sprintf("\\u%04x", int64(r))
		}
		buffer.WriteString(cvt)
	}
	return buffer.Bytes()
}
