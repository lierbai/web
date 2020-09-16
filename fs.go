package web

import (
	"net/http"
	"os"
)

type onlyfilesFS struct {
	fs http.FileSystem
}

type neuteredReaddirFile struct {
	http.File
}

// Dir 返回一个可以被http.FileServer()内部使用的http.Filesystem.被用于router.Static().
// 假如listDirectory为true,那么它的工作方式与http.Dir()相同. 否则将返回一个.
func Dir(root string, listDirectory bool) http.FileSystem {
	fs := http.Dir(root)
	if listDirectory {
		return fs
	}
	return &onlyfilesFS{fs}
}

// Open 打开名为name的http.Filesystem(如果符合对应格式).
func (fs onlyfilesFS) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return neuteredReaddirFile{f}, nil
}

// Readdir 覆盖http.File的默认实现.
func (f neuteredReaddirFile) Readdir(count int) ([]os.FileInfo, error) {
	// 禁用目录列表
	return nil, nil
}
