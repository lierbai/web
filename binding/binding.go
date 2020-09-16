package binding

import "net/http"

const (
	MIMEJSON              = "application/json"
	MIMEHTML              = "text/html"
	MIMEXML               = "application/xml"
	MIMEXML2              = "text/xml"
	MIMEPlain             = "text/plain"
	MIMEPOSTForm          = "application/x-www-form-urlencoded"
	MIMEMultipartPOSTForm = "multipart/form-data"
)

// Binding 绑定请求中的数据所需实现的接口(如JSON请求体、查询参数或表单POST)
type Binding interface {
	Name() string
	Bind(*http.Request, interface{}) error
}

// BindingBody 添加BindBody方法到Binding. BindBody与Bind类似,但它从提供的bytes读取,而不是req.Body.
type BindingBody interface {
	Binding
	BindBody([]byte, interface{}) error
}

// BindingUri 添加BindUri方法到Binding. BindUri与Bind类似,但它读取Params
type BindingUri interface {
	Name() string
	BindUri(map[string][]string, interface{}) error
}

// StructValidator 是需要实现的最小接口,以便用作验证中枢,以确保请求的正确性.
type StructValidator interface {
	ValidateStruct(interface{}) error // 可接收任何类型,即使配置不正确,也不引发异常.
	Engine() interface{}              // 提供支持的基础验证器
}

// Validator 实现StructValidator接口的默认验证器.
var Validator StructValidator = &defaultValidator{}

// 实现Binding接口,并可用于将请求中的数据绑定到struct实例
var (
	Form          = formBinding{}
	FormPost      = formPostBinding{}
	FormMultipart = formMultipartBinding{}
	Header        = headerBinding{}
	JSON          = jsonBinding{}
	Query         = queryBinding{}
	Uri           = uriBinding{}
	XML           = xmlBinding{}
)

// Default 根据HTTP方法和内容类型返回适当的添加BindUri方法到Binding实例.
func Default(method, contentType string) Binding {
	if method == http.MethodGet {
		return Form
	}
	switch contentType {
	case MIMEJSON:
		return JSON
	case MIMEXML, MIMEXML2:
		return XML
	case MIMEMultipartPOSTForm:
		return FormMultipart
	default: // case MIMEPOSTForm:
		return Form
	}
}

func validate(obj interface{}) error {
	if Validator == nil {
		return nil
	}
	return Validator.ValidateStruct(obj)
}
