package gomini

import (
	"github.com/spf13/afero"
	"os"
	"time"
	"syscall"
	"path/filepath"
	"errors"
	"strings"
	"io"
	"sync/atomic"
)

var errOnlyAbsPath = errors.New("only absolute paths are allowed")

type exportFs struct {
	root *exportFile
}

func newExportFs() *exportFs {
	root := &exportFile{
		name:     "",
		dir:      true,
		children: make([]*exportFile, 0),
		time:     time.Now(),
		size:     0,
	}

	return &exportFs{
		root: root,
	}
}

func (e *exportFs) Create(name string) (afero.File, error) {
	return nil, syscall.EPERM
}

func (e *exportFs) Mkdir(name string, perm os.FileMode) error {
	return syscall.EPERM
}

func (e *exportFs) MkdirAll(path string, perm os.FileMode) error {
	return syscall.EPERM
}

func (e *exportFs) Open(name string) (afero.File, error) {
	return e.OpenFile(name, os.O_RDONLY, os.ModePerm)
}

func (e *exportFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, syscall.EPERM
	}

	if !filepath.IsAbs(name) {
		return nil, errOnlyAbsPath
	}

	segments := strings.Split(name, "/")

	entry := e.root
	for segIndex := 1; segIndex < len(segments); segIndex++ {
		for _, child := range entry.children {
			if child.Name() != segments[segIndex] {
				continue
			}

			if !child.dir {
				if segIndex != len(segments)-1 {
					return nil, os.ErrNotExist
				}
			}

			if segIndex == len(segments)-1 {
				return child, nil
			}

			entry = child
			break
		}
	}
	return nil, os.ErrNotExist
}

func (e *exportFs) Remove(name string) error {
	return syscall.EPERM
}

func (e *exportFs) RemoveAll(path string) error {
	return syscall.EPERM
}

func (e *exportFs) Rename(oldname, newname string) error {
	return syscall.EPERM
}

func (e *exportFs) Stat(name string) (os.FileInfo, error) {
	f, err := e.Open(name)
	if err != nil {
		return nil, err
	}
	return f.Stat()
}

func (e *exportFs) Name() string {
	return "exportfs"
}

func (e *exportFs) Chmod(name string, mode os.FileMode) error {
	return syscall.EPERM
}

func (e *exportFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return syscall.EPERM
}

type exportFile struct {
	name     string
	size     int64
	time     time.Time
	dir      bool
	children []*exportFile
	content  []byte
	offset   int64
	fileInfo *exportFileInfo
	module   Module
}

func (e *exportFile) createFile(name string, content []byte, module Module) error {
	if !e.dir {
		return os.ErrPermission
	}

	fileInfo := &exportFileInfo{
		name: name,
		time: time.Now(),
		size: int64(len(content)),
		dir:  false,
	}

	file := &exportFile{
		name:     name,
		offset:   0,
		dir:      false,
		size:     fileInfo.size,
		content:  content,
		time:     fileInfo.time,
		children: nil,
		fileInfo: fileInfo,
		module:   module,
	}

	e.children = append(e.children, file)
	return nil
}

func (e *exportFile) createFolder(name string) (*exportFile, error) {
	if !e.dir {
		return nil, os.ErrPermission
	}

	fileInfo := &exportFileInfo{
		name: name,
		time: time.Now(),
		size: 0,
		dir:  true,
	}

	folder := &exportFile{
		name:     name,
		offset:   0,
		dir:      true,
		size:     0,
		content:  nil,
		time:     fileInfo.time,
		children: make([]*exportFile, 0),
		fileInfo: fileInfo,
	}

	e.children = append(e.children, folder)
	return folder, nil
}

func (e *exportFile) Close() error {
	return syscall.EPERM
}

func (e *exportFile) Read(p []byte) (n int, err error) {
	read, err := e.ReadAt(p, e.offset)
	if err != nil {
		return 0, err
	}
	e.offset += int64(read)
	return read, nil
}

func (e *exportFile) ReadAt(p []byte, off int64) (n int, err error) {
	length := len(p)
	if length > len(e.content)-int(e.offset)-int(off) {
		length = len(e.content) - int(e.offset) - int(off)
	}

	copy(p[:length], e.content[e.offset:int(e.offset)+length])
	e.offset += int64(length)
	return length, nil
}

func (e *exportFile) Seek(offset int64, whence int) (int64, error) {
	if e.dir {
		return 0, os.ErrPermission
	}
	switch whence {
	case io.SeekStart:
		atomic.StoreInt64(&e.offset, offset)
	case io.SeekCurrent:
		e.offset += offset
	case io.SeekEnd:
		e.offset = e.fileInfo.Size() + offset
	}
	return offset, nil
}

func (e *exportFile) Write(p []byte) (n int, err error) {
	return 0, syscall.EPERM
}

func (e *exportFile) WriteAt(p []byte, off int64) (n int, err error) {
	return 0, syscall.EPERM
}

func (e *exportFile) Name() string {
	return e.name
}

func (e *exportFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, os.ErrPermission
}

func (e *exportFile) Readdirnames(n int) ([]string, error) {
	return nil, os.ErrPermission
}

func (e *exportFile) Stat() (os.FileInfo, error) {
	return e.fileInfo, nil
}

func (e *exportFile) Sync() error {
	return syscall.EPERM
}

func (e *exportFile) Truncate(size int64) error {
	return syscall.EPERM
}

func (e *exportFile) WriteString(s string) (ret int, err error) {
	return 0, syscall.EPERM
}

type exportFileInfo struct {
	name string
	size int64
	time time.Time
	dir  bool
}

func (e *exportFileInfo) Name() string {
	return e.name
}

func (e *exportFileInfo) Size() int64 {
	return e.size
}

func (e *exportFileInfo) Mode() os.FileMode {
	return os.ModePerm
}

func (e *exportFileInfo) ModTime() time.Time {
	return e.time
}

func (e *exportFileInfo) IsDir() bool {
	return e.dir
}

func (e *exportFileInfo) Sys() interface{} {
	return nil
}
