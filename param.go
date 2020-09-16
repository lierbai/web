package web

// Param 单个URL参数,由键和值组成.
type Param struct {
	Key   string
	Value string
}

// Params Param切片,由路由返回.(支持索引)
type Params []Param

// Get 返回指定键名匹配到的首个值.不存在则返回nil
func (ps Params) Get(name string) (string, bool) {
	for _, entry := range ps {
		if entry.Key == name {
			return entry.Value, true
		}
	}
	return "", false
}

// ByName 返回指定键名匹配到的首个值.不存在则返回nil
func (ps Params) ByName(name string) (va string) {
	va, _ = ps.Get(name)
	return
}
