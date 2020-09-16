package bytesconv

import (
	"reflect"
	"unsafe"
)

// StringToBytes 在不分配内存的情况下将字符串转换为字节片.
func StringToBytes(s string) (b []byte) {
	sh := *(*reflect.StringHeader)(unsafe.Pointer(&s))
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	bh.Data, bh.Len, bh.Cap = sh.Data, sh.Len, sh.Len
	return b
}

// BytesToString 在不分配内存的情况下将字节片转换为字符串.
func BytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
