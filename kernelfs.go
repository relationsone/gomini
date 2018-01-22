package gomini

import (
	"github.com/spf13/afero"
	"os"
	"time"
	"syscall"
)

type kernelfs struct {
}

func newKernelfs(bundle Bundle) *kernelfs {
	return &kernelfs{	}
}

func (fs *kernelfs) Open(name string) (afero.File, error) {
	return fs.OpenFile(name, os.O_RDONLY, os.ModePerm)
}

func (fs *kernelfs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	panic("implement me")
}

func (fs *kernelfs) Stat(name string) (os.FileInfo, error) {
	panic("implement me")
}

func (fs *kernelfs) Name() string {
	return "kernelfs"
}

func (fs *kernelfs) Create(name string) (afero.File, error) {
	return nil, syscall.EPERM
}

func (fs *kernelfs) Mkdir(name string, perm os.FileMode) error {
	return syscall.EPERM
}

func (fs *kernelfs) MkdirAll(path string, perm os.FileMode) error {
	return syscall.EPERM
}

func (fs *kernelfs) Remove(name string) error {
	return syscall.EPERM
}

func (fs *kernelfs) RemoveAll(path string) error {
	return syscall.EPERM
}

func (fs *kernelfs) Rename(oldname, newname string) error {
	return syscall.EPERM
}

func (fs *kernelfs) Chmod(name string, mode os.FileMode) error {
	return syscall.EPERM
}

func (fs *kernelfs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return syscall.EPERM
}
