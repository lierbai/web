package web

import (
	"net/url"
	"strings"
	"unicode"
)

type nodeType uint8

const (
	static nodeType = iota // default
	root
	param
	catchAll
)

type node struct {
	path      string
	indices   string
	children  []*node
	handlers  HandlersChain
	priority  uint32
	nType     nodeType
	maxParams uint8
	wildChild bool
	fullPath  string
}

// increments 给定子节点的优先级,必要时重新排序.
func (n *node) incrementChildPrio(pos int) int {
	cs := n.children
	cs[pos].priority++
	prio := cs[pos].priority

	// 调整位置 (向前移动)
	newPos := pos
	for ; newPos > 0 && cs[newPos-1].priority < prio; newPos-- {
		// 交换节点位置
		cs[newPos-1], cs[newPos] = cs[newPos], cs[newPos-1]
	}

	// 生成新的索引字符串
	if newPos != pos {
		n.indices = n.indices[:newPos] + // 前缀未更改, 可能为空
			n.indices[pos:pos+1] + // 被移动的索引字符
			n.indices[newPos:pos] + n.indices[pos+1:] // 剩余字符(pos外的)
	}

	return newPos
}

// addRoute 将具有给定句柄的节点添加到指定路径(非并发安全).
func (n *node) addRoute(path string, handlers HandlersChain) {
	fullPath := path
	n.priority++
	numParams := countParams(path)

	// 如果树为空
	if len(n.path) == 0 && len(n.children) == 0 {
		n.insertChild(numParams, path, fullPath, handlers)
		n.nType = root
		return
	}

	parentFullPathIndex := 0

walk:
	for {
		// 更新当前节点的 maxParams
		if numParams > n.maxParams {
			n.maxParams = numParams
		}

		// 寻找最长的公共前缀.公共前缀不包含':'和'*',因为这些是不明确的.
		i := longestCommonPrefix(path, n.path)

		// 分割边缘
		if i < len(n.path) {
			child := node{
				path:      n.path[i:],
				wildChild: n.wildChild,
				indices:   n.indices,
				children:  n.children,
				handlers:  n.handlers,
				priority:  n.priority - 1,
				fullPath:  n.fullPath,
			}

			// 更新maxparms (所有子级的最大值)
			for _, v := range child.children {
				if v.maxParams > child.maxParams {
					child.maxParams = v.maxParams
				}
			}

			n.children = []*node{&child}
			// []byte  有关正确的unicode字符转换, see #65
			n.indices = string([]byte{n.path[i]})
			n.path = path[:i]
			n.handlers = nil
			n.wildChild = false
			n.fullPath = fullPath[:parentFullPathIndex+i]
		}

		// 使新节点成为此节点的子节点
		if i < len(path) {
			path = path[i:]

			if n.wildChild {
				parentFullPathIndex += len(n.path)
				n = n.children[0]
				n.priority++

				// 更新当前节点的 maxParams
				if numParams > n.maxParams {
					n.maxParams = numParams
				}
				numParams--

				// 检查通配符是否匹配
				if len(path) >= len(n.path) && n.path == path[:len(n.path)] {
					// 检查较长的通配符，例如：name和：names
					if len(n.path) >= len(path) || path[len(n.path)] == '/' {
						continue walk
					}
				}

				pathSeg := path
				if n.nType != catchAll {
					pathSeg = strings.SplitN(path, "/", 2)[0]
				}
				prefix := fullPath[:strings.Index(fullPath, pathSeg)] + n.path
				panic("新路径'" + fullPath + "'中的'" + pathSeg + "'与现有前缀'" + prefix + "'中通配符'" + n.path + "'冲突")
			}

			c := path[0]

			// 参数后斜杠
			if n.nType == param && c == '/' && len(n.children) == 1 {
				parentFullPathIndex += len(n.path)
				n = n.children[0]
				n.priority++
				continue walk
			}

			// 检查是否存在具有下一个路径字节的子级
			for i, max := 0, len(n.indices); i < max; i++ {
				if c == n.indices[i] {
					parentFullPathIndex += len(n.path)
					i = n.incrementChildPrio(i)
					n = n.children[i]
					continue walk
				}
			}

			// 否则就插入
			if c != ':' && c != '*' {
				// []byte 有关正确的unicode字符转换, see #65
				n.indices += string([]byte{c})
				child := &node{
					maxParams: numParams,
					fullPath:  fullPath,
				}
				n.children = append(n.children, child)
				n.incrementChildPrio(len(n.indices) - 1)
				n = child
			}
			n.insertChild(numParams, path, fullPath, handlers)
			return
		}

		// 否则 当前节点的handle
		if n.handlers != nil {
			panic("已为路径 '" + fullPath + "'注册处理程序")
		}
		n.handlers = handlers
		return
	}
}

func (n *node) insertChild(numParams uint8, path string, fullPath string, handlers HandlersChain) {
	for numParams > 0 {
		// 查找前缀直到第一个通配符
		wildcard, i, valid := findWildcard(path)
		if i < 0 { // 找不到通配符
			break
		}

		// 通配符名称不能包含 ':' 和 '*'
		if !valid {
			panic("每个路径段只允许一个通配符,发现: '" + wildcard + "'在'" + fullPath + "'路径中")
		}

		// 检查通配符是否有名称
		if len(wildcard) < 2 {
			panic("通配符必须在路径'" + fullPath + "'中使用非空名称命名")
		}

		// 检查此节点是否有现有的子节点,如果在此处插入通配符,则无法访问这些子节点
		if len(n.children) > 0 {
			panic("通配符段'" + wildcard + "'与路径'" + fullPath + "'中的现有子级冲突")
		}

		if wildcard[0] == ':' { // 路径内参数
			if i > 0 {
				// 在当前通配符之前插入前缀
				n.path = path[:i]
				path = path[i:]
			}

			n.wildChild = true
			child := &node{
				nType:     param,
				path:      wildcard,
				maxParams: numParams,
				fullPath:  fullPath,
			}
			n.children = []*node{child}
			n = child
			n.priority++
			numParams--

			// 如果路径不以通配符结尾,则会有另一个以"/"开头的非通配符子路径
			if len(wildcard) < len(path) {
				path = path[len(wildcard):]

				child := &node{
					maxParams: numParams,
					priority:  1,
					fullPath:  fullPath,
				}
				n.children = []*node{child}
				n = child
				continue
			}
			// 否则我们结束了。将handle插入新叶子节点
			n.handlers = handlers
			return
		}

		// 捕获所有
		if i+len(wildcard) != len(path) || numParams > 1 {
			panic("只允许在路径'" + fullPath + "'中的结尾处捕获所有路由")
		}

		if len(n.path) > 0 && n.path[len(n.path)-1] == '/' {
			panic("捕获与路径'" + fullPath + "'中路径段 根节点的现有句柄的所有冲突")
		}

		// '/'的当前固定宽度为1
		i--
		if path[i] != '/' {
			panic("在路径'" + fullPath + "'中全部捕获前不要使用'/'")
		}

		n.path = path[:i]

		// 第一个节点:路径为空的catchAll节点
		child := &node{
			wildChild: true,
			nType:     catchAll,
			maxParams: 1,
			fullPath:  fullPath,
		}
		// 更新父节点的maxparms
		if n.maxParams < 1 {
			n.maxParams = 1
		}
		n.children = []*node{child}
		n.indices = string('/')
		n = child
		n.priority++

		// 第二个节点:保存变量的节点
		child = &node{
			path:      path[i:],
			nType:     catchAll,
			maxParams: 1,
			handlers:  handlers,
			priority:  1,
			fullPath:  fullPath,
		}
		n.children = []*node{child}

		return
	}

	// 如果没有找到通配符，只需插入路径和句柄
	n.path = path
	n.handlers = handlers
	n.fullPath = fullPath
}

// nodeValue 用来保存 (*Node).getValue 方法的返回值
type nodeValue struct {
	handlers HandlersChain
	params   Params
	tsr      bool
	fullPath string
}

// getValue 返回注册到给定路径 (key)的句柄. 通配符的值将保存到映射中.
// 如果找不到句柄,存在没有结尾斜杠的句柄.提出TSR(尾部斜杠重定向)建议
func (n *node) getValue(path string, po Params, unescape bool) (value nodeValue) {
	value.params = po
walk: // 在树上运行的外循环
	for {
		prefix := n.path
		if path == prefix {
			// 我们应该到达了包含句柄的节点.检查此节点是否注册了句柄.
			if value.handlers = n.handlers; value.handlers != nil {
				value.fullPath = n.fullPath
				return
			}

			if path == "/" && n.wildChild && n.nType != root {
				value.tsr = true
				return
			}

			// 找不到handle.检查此路径的handle+TSR建议在尾部添加斜杠
			indices := n.indices
			for i, max := 0, len(indices); i < max; i++ {
				if indices[i] == '/' {
					n = n.children[i]
					value.tsr = (len(n.path) == 1 && n.handlers != nil) ||
						(n.nType == catchAll && n.children[0].handlers != nil)
					return
				}
			}

			return
		}

		if len(path) > len(prefix) && path[:len(prefix)] == prefix {
			path = path[len(prefix):]
			// 如果这个节点没有通配符（param或catchAll）子节点,我们可以查找下一个子节点并继续沿着树向下走
			if !n.wildChild {
				c := path[0]
				indices := n.indices
				for i, max := 0, len(indices); i < max; i++ {
					if c == indices[i] {
						n = n.children[i]
						continue walk
					}
				}

				// Nothing found.
				// 如果路径存在叶子节点,可以建议重定向到不带斜杠的同一个URL.
				value.tsr = path == "/" && n.handlers != nil
				return
			}

			// 处理通配符子节点
			n = n.children[0]
			switch n.nType {
			case param:
				// 寻找参数终止符('/'或路径结尾)
				end := 0
				for end < len(path) && path[end] != '/' {
					end++
				}

				// 保存参数值
				if cap(value.params) < int(n.maxParams) {
					value.params = make(Params, 0, n.maxParams)
				}
				i := len(value.params)
				value.params = value.params[:i+1] // 在预先分配的容量内扩展切片
				value.params[i].Key = n.path[1:]
				val := path[:end]
				if unescape {
					var err error
					if value.params[i].Value, err = url.QueryUnescape(val); err != nil {
						value.params[i].Value = val // 回退,在错误的情况下
					}
				} else {
					value.params[i].Value = val
				}

				// 我们需要更深入树
				if end < len(path) {
					if len(n.children) > 0 {
						path = path[end:]
						n = n.children[0]
						continue walk
					}

					// ... 但是并不行
					value.tsr = len(path) == end+1
					return
				}

				if value.handlers = n.handlers; value.handlers != nil {
					value.fullPath = n.fullPath
					return
				}
				if len(n.children) == 1 {
					// 找不到handle.检查此路径的handle+TSR建议在尾部添加斜杠
					n = n.children[0]
					value.tsr = n.path == "/" && n.handlers != nil
				}
				return

			case catchAll:
				// 保存参数值
				if cap(value.params) < int(n.maxParams) {
					value.params = make(Params, 0, n.maxParams)
				}
				i := len(value.params)
				value.params = value.params[:i+1] // 在预先分配的容量内扩展切片
				value.params[i].Key = n.path[2:]
				if unescape {
					var err error
					if value.params[i].Value, err = url.QueryUnescape(path); err != nil {
						value.params[i].Value = path // 回退,在错误的情况下
					}
				} else {
					value.params[i].Value = path
				}

				value.handlers = n.handlers
				value.fullPath = n.fullPath
				return

			default:
				panic("无效的节点类型")
			}
		}
		// Nothing found.
		// 如果该路径存在子叶,可以建议在末尾添加斜杠重定向到同一个URL
		value.tsr = (path == "/") ||
			(len(prefix) == len(path)+1 && prefix[len(path)] == '/' &&
				path == prefix[:len(prefix)-1] && n.handlers != nil)
		return
	}
}

// findCaseInsensitivePath 对路径进行查找(不区分大小写),并尝试查找处理程序.
// 返回大小写更正的路径和bool(查找是否成功).它还可以选择性地修复尾部斜杠.
func (n *node) findCaseInsensitivePath(path string, fixTrailingSlash bool) (ciPath []byte, found bool) {
	ciPath = make([]byte, 0, len(path)+1) // 预分配足够的内存

	// 在树上面运行的循环遍历
	for len(path) >= len(n.path) && strings.EqualFold(path[:len(n.path)], n.path) {
		path = path[len(n.path):]
		ciPath = append(ciPath, n.path...)

		if len(path) == 0 {
			// 应该到达了包含句柄的节点.
			// 检查此节点是否注册了handle	.
			if n.handlers != nil {
				return ciPath, true
			}

			// No handle found.
			// 尝试通过添加尾随斜杠来修复路径
			if fixTrailingSlash {
				for i := 0; i < len(n.indices); i++ {
					if n.indices[i] == '/' {
						n = n.children[i]
						if (len(n.path) == 1 && n.handlers != nil) ||
							(n.nType == catchAll && n.children[0].handlers != nil) {
							return append(ciPath, '/'), true
						}
						return
					}
				}
			}
			return
		}

		// 如果这个节点没有通配符（param或catchAll）子节点,我们可以查找下一个子节点并继续沿着树向下走
		if !n.wildChild {
			r := unicode.ToLower(rune(path[0]))
			for i, index := range n.indices {
				// 必须使用递归方法.
				// 因为索引和ToLower(index)都可能存在.我们两个都要检查.
				if r == unicode.ToLower(index) {
					out, found := n.children[i].findCaseInsensitivePath(path, fixTrailingSlash)
					if found {
						return append(ciPath, out...), true
					}
				}
			}

			// Nothing found. 如果路径存在叶，我们可以建议重定向到不带斜杠的URL
			found = fixTrailingSlash && path == "/" && n.handlers != nil
			return
		}

		n = n.children[0]
		switch n.nType {
		case param:
			// 寻找参数终止符('/'或路径结尾)
			end := 0
			for end < len(path) && path[end] != '/' {
				end++
			}

			// 将参数值添加到不区分大小写的路径
			ciPath = append(ciPath, path[:end]...)

			// 我们需要更深入树
			if end < len(path) {
				if len(n.children) > 0 {
					path = path[end:]
					n = n.children[0]
					continue
				}

				// ... 但是并不行
				if fixTrailingSlash && len(path) == end+1 {
					return ciPath, true
				}
				return
			}

			if n.handlers != nil {
				return ciPath, true
			}
			if fixTrailingSlash && len(n.children) == 1 {
				// 找不到handle.检查此路径的handle并提出TSR建议
				n = n.children[0]
				if n.path == "/" && n.handlers != nil {
					return append(ciPath, '/'), true
				}
			}
			return

		case catchAll:
			return append(ciPath, path...), true

		default:
			panic("无效的节点类型")
		}
	}

	// Nothing found.
	// 尝试通过添加|删除尾部斜杠来修复路径(由fixTrailingSlash 控制)
	if fixTrailingSlash {
		if path == "/" {
			return ciPath, true
		}
		if len(path)+1 == len(n.path) && n.path[len(path)] == '/' &&
			strings.EqualFold(path, n.path[:len(path)]) &&
			n.handlers != nil {
			return append(ciPath, n.path...), true
		}
	}
	return
}

/*  其他工具函数 */
func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func longestCommonPrefix(a, b string) int {
	i := 0
	max := min(len(a), len(b))
	for i < max && a[i] == b[i] {
		i++
	}
	return i
}

func countParams(path string) uint8 {
	var n uint
	for i := 0; i < len(path); i++ {
		if path[i] == ':' || path[i] == '*' {
			n++
		}
	}
	if n >= 255 {
		return 255
	}
	return uint8(n)
}

// 搜索通配符段并检查名称中是否存在无效字符
// 如果未发现通配符冲突,则返回-1作为索引
func findWildcard(path string) (wildcard string, i int, valid bool) {
	// 开始搜索
	for start, c := range []byte(path) {
		// 通配符以 ':' (param) or '*'开头 (全部拦截)
		if c != ':' && c != '*' {
			continue
		}

		// 查找结尾并检查无效字符
		valid = true
		for end, c := range []byte(path[start+1:]) {
			switch c {
			case '/':
				return path[start : start+1+end], start, valid
			case ':', '*':
				valid = false
			}
		}
		return path[start:], start, valid
	}
	return "", -1, false
}
