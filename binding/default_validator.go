package binding

import (
	"reflect"
	"sync"

	"github.com/go-playground/validator/v10"
)

type defaultValidator struct {
	once     sync.Once
	validate *validator.Validate
}

var _ StructValidator = &defaultValidator{}

// ValidateStruct 能接收任何类型并执行验证(只执行struct或指向struct的指针).
func (v *defaultValidator) ValidateStruct(obj interface{}) error {
	value := reflect.ValueOf(obj)
	valueType := value.Kind()
	// If 值类型为指针,获取指针指向的数据的值类型
	if valueType == reflect.Ptr {
		valueType = value.Elem().Kind()
	}
	// 如果最终值类型为Struct.执行验证
	if valueType == reflect.Struct {
		v.lazyinit()
		if err := v.validate.Struct(obj); err != nil {
			// 结构无效或验证失败,返回错误描述
			return err
		}
	}
	// 其他情况返回 nil
	return nil
}

// Engine 返回为默认验证器提供支持的基础引擎.这有利于注册自定义验证器或结构验证
// 更多信息,请参阅相关文档 - https://godoc.org/gopkg.in/go-playground/validator.v8
func (v *defaultValidator) Engine() interface{} {
	v.lazyinit()
	return v.validate
}

func (v *defaultValidator) lazyinit() {
	v.once.Do(func() {
		v.validate = validator.New()
		v.validate.SetTagName("binding")
	})
}
