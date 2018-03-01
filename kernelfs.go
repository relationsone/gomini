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

func IsKernelFile(filesystem afero.Fs, name string) bool {
	info, err := filesystem.Stat(name)
	if err != nil {
		return false
	}
	_, ok := info.(*kernelFileInfo)
	return ok
}

type Any interface{}

type KernelSyscall func(caller Bundle) interface{}

type kernelFs struct {
	root *kernelFile
}

func newKernelFs() *kernelFs {
	root := &kernelFile{
		name:     "",
		dir:      true,
		children: make([]*kernelFile, 0),
		time:     time.Now(),
		size:     0,
	}

	return &kernelFs{
		root: root,
	}
}

func (k *kernelFs) Create(name string) (afero.File, error) {
	return nil, syscall.EPERM
}

func (k *kernelFs) Mkdir(name string, perm os.FileMode) error {
	return syscall.EPERM
}

func (k *kernelFs) MkdirAll(path string, perm os.FileMode) error {
	return syscall.EPERM
}

func (k *kernelFs) Open(name string) (afero.File, error) {
	return k.OpenFile(name, os.O_RDONLY, os.ModePerm)
}

func (k *kernelFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, syscall.EPERM
	}

	return k.find(name)
}

func (k *kernelFs) Remove(name string) error {
	return syscall.EPERM
}

func (k *kernelFs) RemoveAll(path string) error {
	return syscall.EPERM
}

func (k *kernelFs) Rename(oldname, newname string) error {
	return syscall.EPERM
}

func (k *kernelFs) Stat(name string) (os.FileInfo, error) {
	f, err := k.find(name)
	if err != nil {
		return nil, err
	}
	return f.Stat()
}

func (k *kernelFs) Name() string {
	return "kernelfs"
}

func (k *kernelFs) Chmod(name string, mode os.FileMode) error {
	return syscall.EPERM
}

func (k *kernelFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return syscall.EPERM
}

func (k *kernelFs) find(name string) (afero.File, error) {
	if !filepath.IsAbs(name) {
		return nil, errOnlyAbsPath
	}

	segments := strings.Split(name, "/")

	entry := k.root
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

type kernelFile struct {
	name     string
	size     int64
	time     time.Time
	dir      bool
	children []*kernelFile
	content  []byte
	offset   int64
	fileInfo *kernelFileInfo
	syscall  KernelSyscall
}

func (k *kernelFile) createFile(name string, content []byte, syscall KernelSyscall) error {
	if !k.dir {
		return os.ErrPermission
	}

	fileInfo := &kernelFileInfo{
		name:    name,
		time:    time.Now(),
		size:    int64(len(content)),
		dir:     false,
		syscall: syscall,
	}

	file := &kernelFile{
		name:     name,
		offset:   0,
		dir:      false,
		size:     fileInfo.size,
		content:  content,
		time:     fileInfo.time,
		children: nil,
		fileInfo: fileInfo,
		syscall:  syscall,
	}

	k.children = append(k.children, file)
	return nil
}

func (k *kernelFile) createFolder(name string) (*kernelFile, error) {
	if !k.dir {
		return nil, os.ErrPermission
	}

	fileInfo := &kernelFileInfo{
		name: name,
		time: time.Now(),
		size: 0,
		dir:  true,
	}

	folder := &kernelFile{
		name:     name,
		offset:   0,
		dir:      true,
		size:     0,
		content:  nil,
		time:     fileInfo.time,
		children: make([]*kernelFile, 0),
		fileInfo: fileInfo,
	}

	k.children = append(k.children, folder)
	return folder, nil
}

func (k *kernelFile) Close() error {
	return syscall.EPERM
}

func (k *kernelFile) Read(p []byte) (n int, err error) {
	read, err := k.ReadAt(p, k.offset)
	if err != nil {
		return 0, err
	}
	k.offset += int64(read)
	return read, nil
}

func (k *kernelFile) ReadAt(p []byte, off int64) (n int, err error) {
	length := len(p)
	if length > len(k.content)-int(k.offset)-int(off) {
		length = len(k.content) - int(k.offset) - int(off)
	}

	copy(p[:length], k.content[k.offset:int(k.offset)+length])
	k.offset += int64(length)
	return length, nil
}

func (k *kernelFile) Seek(offset int64, whence int) (int64, error) {
	if k.dir {
		return 0, os.ErrPermission
	}
	switch whence {
	case io.SeekStart:
		atomic.StoreInt64(&k.offset, offset)
	case io.SeekCurrent:
		k.offset += offset
	case io.SeekEnd:
		k.offset = k.fileInfo.Size() + offset
	}
	return offset, nil
}

func (k *kernelFile) Write(p []byte) (n int, err error) {
	return 0, syscall.EPERM
}

func (k *kernelFile) WriteAt(p []byte, off int64) (n int, err error) {
	return 0, syscall.EPERM
}

func (k *kernelFile) Name() string {
	return k.name
}

func (k *kernelFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, os.ErrPermission
}

func (k *kernelFile) Readdirnames(n int) ([]string, error) {
	return nil, os.ErrPermission
}

func (k *kernelFile) Stat() (os.FileInfo, error) {
	return k.fileInfo, nil
}

func (k *kernelFile) Sync() error {
	return syscall.EPERM
}

func (k *kernelFile) Truncate(size int64) error {
	return syscall.EPERM
}

func (k *kernelFile) WriteString(s string) (ret int, err error) {
	return 0, syscall.EPERM
}

type kernelFileInfo struct {
	name    string
	size    int64
	time    time.Time
	dir     bool
	syscall KernelSyscall
}

func (k *kernelFileInfo) Name() string {
	return k.name
}

func (k *kernelFileInfo) Size() int64 {
	return k.size
}

func (k *kernelFileInfo) Mode() os.FileMode {
	return os.ModePerm
}

func (k *kernelFileInfo) ModTime() time.Time {
	return k.time
}

func (k *kernelFileInfo) IsDir() bool {
	return k.dir
}

func (k *kernelFileInfo) Sys() interface{} {
	return k.syscall
}
