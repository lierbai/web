package web

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// ErrorType 无符号64位错误代码.
type ErrorType uint64

const (
	// ErrorTypeBind 在Context.Bind()失败时使用.
	ErrorTypeBind ErrorType = 1 << 63 //
	// ErrorTypeRender 在Context.Render()失败时使用.
	ErrorTypeRender ErrorType = 1 << 62 //
	// ErrorTypePrivate 表示私有错误.
	ErrorTypePrivate ErrorType = 1 << 0 //
	// ErrorTypePublic 表示公共错误.
	ErrorTypePublic ErrorType = 1 << 1 //
	// ErrorTypeAny 表示其他错误.
	ErrorTypeAny ErrorType = 1<<64 - 1 //
	// ErrorTypeNu 表示其他错误.
	ErrorTypeNu = 2 //
)

// Error 错误的规范.
type Error struct {
	Err  error
	Type ErrorType
	Meta interface{}
}

type errorMsgs []*Error

var _ error = &Error{}

// SetType 设置错误的类型.
func (msg *Error) SetType(flags ErrorType) *Error {
	msg.Type = flags
	return msg
}

// SetMeta 设置错误的 meta 数据.
func (msg *Error) SetMeta(data interface{}) *Error {
	msg.Meta = data
	return msg
}

// JSON 创建格式正确的JSON
func (msg *Error) JSON() interface{} {
	json := Data{}
	if msg.Meta != nil {
		value := reflect.ValueOf(msg.Meta)
		switch value.Kind() {
		case reflect.Struct:
			return msg.Meta
		case reflect.Map:
			for _, key := range value.MapKeys() {
				json[key.String()] = value.MapIndex(key).Interface()
			}
		default:
			json["meta"] = msg.Meta
		}
	}
	if _, ok := json["error"]; !ok {
		json["error"] = msg.Error()
	}
	return json
}

// MarshalJSON 实现 json.Marshaller 接口.
func (msg *Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(msg.JSON())
}

// Error 实现 error 接口.
func (msg Error) Error() string {
	return msg.Err.Error()
}

// IsType 判断一个错误.
func (msg *Error) IsType(flags ErrorType) bool {
	return (msg.Type & flags) > 0
}

// ByType 返回经过筛选的字节的只读副本.
// ie ByType(gin.ErrorTypePublic) 返回错误类型和类型为ErrorTypePublic.
func (a errorMsgs) ByType(typ ErrorType) errorMsgs {
	if len(a) == 0 {
		return nil
	}
	if typ == ErrorTypeAny {
		return a
	}
	var result errorMsgs
	for _, msg := range a {
		if msg.IsType(typ) {
			result = append(result, msg)
		}
	}
	return result
}

// Last 返回切片中的最后一个错误. 如果数组为空,则返回nil.错误的快捷方式[len(errors)-1].
func (a errorMsgs) Last() *Error {
	if length := len(a); length > 0 {
		return a[length-1]
	}
	return nil
}

// Errors 返回一个数组将所有错误消息.
// Example:
// 		c.Error(errors.New("first"))
// 		c.Error(errors.New("second"))
// 		c.Error(errors.New("third"))
// 		c.Errors.Errors() // == []string{"first", "second", "third"}
func (a errorMsgs) Errors() []string {
	if len(a) == 0 {
		return nil
	}
	errorStrings := make([]string, len(a))
	for i, err := range a {
		errorStrings[i] = err.Error()
	}
	return errorStrings
}

func (a errorMsgs) JSON() interface{} {
	switch len(a) {
	case 0:
		return nil
	case 1:
		return a.Last().JSON()
	default:
		json := make([]interface{}, len(a))
		for i, err := range a {
			json[i] = err.JSON()
		}
		return json
	}
}

// MarshalJSON 实现 json.Marshaller 接口.
func (a errorMsgs) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.JSON())
}

func (a errorMsgs) String() string {
	if len(a) == 0 {
		return ""
	}
	var buffer strings.Builder
	for i, msg := range a {
		fmt.Fprintf(&buffer, "Error #%02d: %s\n", i+1, msg.Err)
		if msg.Meta != nil {
			fmt.Fprintf(&buffer, "     Meta: %v\n", msg.Meta)
		}
	}
	return buffer.String()
}
