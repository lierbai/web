package binding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// EnableDecoderUseNumber 用于调用JSON解码器实例上的UseNumber方法. UseNumber 使解码器将一个数字解组到接口中，作为数字而不是float64.
var EnableDecoderUseNumber = false

// EnableDecoderDisallowUnknownFields 用于调用JSON解码器实例上的DisallowUnknownFields方法. 当目标为结构且输入包含与目标中任何未忽略的导出字段不匹配的对象键时,使解码器返回错误.
var EnableDecoderDisallowUnknownFields = false

type jsonBinding struct{}

func (jsonBinding) Name() string {
	return "json"
}

func (jsonBinding) Bind(req *http.Request, obj interface{}) error {
	if req == nil || req.Body == nil {
		return fmt.Errorf("invalid request")
	}
	return decodeJSON(req.Body, obj)
}

func (jsonBinding) BindBody(body []byte, obj interface{}) error {
	return decodeJSON(bytes.NewReader(body), obj)
}

func decodeJSON(r io.Reader, obj interface{}) error {
	decoder := json.NewDecoder(r)
	if EnableDecoderUseNumber {
		decoder.UseNumber()
	}
	if EnableDecoderDisallowUnknownFields {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(obj); err != nil {
		return err
	}
	return validate(obj)
}
