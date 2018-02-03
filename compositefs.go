package gomini

import (
	"github.com/spf13/afero"
	"os"
	"time"
	"path/filepath"
	"strings"
	"errors"
	"io"
	"syscall"
)

const pathSeparator = "/"

type CompositeFs struct {
	base         afero.Fs
	mounts       map[string]afero.Fs
	parents      map[string][]string
	creationTime time.Time
}

func NewCompositeFs(base afero.Fs) *CompositeFs {
	return &CompositeFs{
		base:         base,
		mounts:       make(map[string]afero.Fs),
		parents:      make(map[string][]string),
		creationTime: time.Now(),
	}
}

func (c *CompositeFs) Mount(mount afero.Fs, path string) error {
	// Clean path and make absolute
	path = filepath.Clean(path)
	segm := c.splitPath(path, pathSeparator)
	segm[0] = "" // make absolute
	path = strings.Join(segm, pathSeparator)

	parent := strings.Join(segm[0:len(segm)-1], pathSeparator)
	if parent == "" {
		parent = "/"
	}

	c.parents[parent] = append(c.parents[parent], path)
	c.mounts[path] = mount
	return nil
}

func (c *CompositeFs) Create(name string) (afero.File, error) {
	mount, innerPath := c.findMount(name)
	return mount.Create(innerPath)
}

func (c *CompositeFs) Mkdir(name string, perm os.FileMode) error {
	mount, innerPath := c.findMount(name)
	return mount.Mkdir(innerPath, perm)
}

func (c *CompositeFs) MkdirAll(path string, perm os.FileMode) error {
	mount, innerPath := c.findMount(path)
	return mount.MkdirAll(innerPath, perm)
}

func (c *CompositeFs) Open(name string) (afero.File, error) {
	return c.OpenFile(name, os.O_RDONLY, os.ModePerm)
}

func (c *CompositeFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	mount, innerPath := c.findMount(name)

	file, err := mount.OpenFile(innerPath, flag, perm)
	if err != nil {
		switch err.(type) {
		case *os.PathError:
			children := c.parents[name]
			if children != nil {
				return &compositeShadowFile{
					fs:       c,
					name:     filepath.Base(name),
					path:     name,
					children: children,
				}, nil
			}
		}

		return nil, err
	}

	return &compositeFile{
		fs:        c,
		file:      file,
		path:      name,
		mount:     mount,
		innerPath: innerPath,
	}, nil
}

func (c *CompositeFs) Remove(name string) error {
	mount, innerPath := c.findMount(name)
	return mount.Remove(innerPath)
}

func (c *CompositeFs) RemoveAll(path string) error {
	mount, innerPath := c.findMount(path)
	return mount.RemoveAll(innerPath)
}

func (c *CompositeFs) Rename(oldname, newname string) error {
	oldMount, oldInnerPath := c.findMount(oldname)
	newMount, newInnerPath := c.findMount(newname)

	if oldMount != newMount {
		return errors.New("filesystem mounts are not compatible to rename a file")
	}

	return oldMount.Rename(oldInnerPath, newInnerPath)
}

func (c *CompositeFs) Stat(name string) (os.FileInfo, error) {
	file, err := c.Open(name)
	if err != nil {
		return nil, err
	}
	return file.Stat()
}

func (c *CompositeFs) Name() string {
	return "CompositeFs"
}

func (c *CompositeFs) Chmod(name string, mode os.FileMode) error {
	mount, innerPath := c.findMount(name)
	return mount.Chmod(innerPath, mode)
}

func (c *CompositeFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	mount, innerPath := c.findMount(name)
	return mount.Chtimes(innerPath, atime, mtime)
}

func (c *CompositeFs) findMount(path string) (afero.Fs, string) {
	path = filepath.Clean(path)
	segs := c.splitPath(path, pathSeparator)
	length := len(segs)
	for i := length; i > 0; i-- {
		mountPath := strings.Join(segs[0:i], pathSeparator)
		if fs, ok := c.mounts[mountPath]; ok {
			return fs, "/" + strings.Join(segs[i:length], pathSeparator)
		}
	}
	return c.base, path
}

// SplitPath splits the given path in segments:
// 	"/" 				-> []string{""}
//	"./file" 			-> []string{".", "file"}
//	"file" 				-> []string{".", "file"}
//	"/usr/src/linux/" 	-> []string{"", "usr", "src", "linux"}
// The returned slice of path segments consists of one more more segments.
func (c *CompositeFs) splitPath(path string, sep string) []string {
	path = strings.TrimSpace(path)
	path = strings.TrimSuffix(path, sep)
	if path == "" { // was "/"
		return []string{""}
	}
	if path == "." {
		return []string{"."}
	}

	if len(path) > 0 && !strings.HasPrefix(path, sep) && !strings.HasPrefix(path, "."+sep) {
		path = "./" + path
	}
	parts := strings.Split(path, sep)

	return parts
}

type compositeFile struct {
	fs        *CompositeFs
	file      afero.File
	path      string
	mount     afero.Fs
	innerPath string
}

func (c *compositeFile) Close() error {
	return c.file.Close()
}

func (c *compositeFile) Read(p []byte) (n int, err error) {
	return c.file.Read(p)
}

func (c *compositeFile) ReadAt(p []byte, off int64) (n int, err error) {
	return c.file.ReadAt(p, off)
}

func (c *compositeFile) Seek(offset int64, whence int) (int64, error) {
	return c.file.Seek(offset, whence)
}

func (c *compositeFile) Write(p []byte) (n int, err error) {
	return c.file.Write(p)
}

func (c *compositeFile) WriteAt(p []byte, off int64) (n int, err error) {
	return c.file.WriteAt(p, off)
}

func (c *compositeFile) Name() string {
	return c.path
}

func (c *compositeFile) Readdir(count int) ([]os.FileInfo, error) {
	fileInfos, err := c.file.Readdir(count)
	if err != nil {
		return nil, err
	}

	if len(fileInfos) == count {
		return fileInfos, nil
	}

	for parent := range c.fs.parents {
		if !strings.HasPrefix(parent, c.path) {
			continue
		}

		child := strings.Replace(parent, c.path, "", -1)
		if len(strings.Split(child, "/")) == 1 {
			name := filepath.Join(c.path, child)
			fileInfo, err := c.fs.Stat(name)
			if err != nil {
				return nil, err
			}
			fileInfos = append(fileInfos, fileInfo)

			if len(fileInfos) == count {
				return fileInfos, nil
			}
		}
	}

	if len(fileInfos) < count {
		return fileInfos, io.EOF
	}

	return fileInfos, nil
}

func (c *compositeFile) Readdirnames(n int) ([]string, error) {
	fileInfos, err := c.Readdir(n)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(fileInfos))
	for i, fi := range fileInfos {
		names[i] = fi.Name()
	}
	return names, nil
}

func (c *compositeFile) Stat() (os.FileInfo, error) {
	fileInfo, err := c.file.Stat()
	if err != nil {
		return nil, err
	}
	return &compositeFileInfo{
		fileInfo: fileInfo,
		name:     filepath.Base(c.path),
	}, nil
}

func (c *compositeFile) Sync() error {
	return c.file.Sync()
}

func (c *compositeFile) Truncate(size int64) error {
	return c.file.Truncate(size)
}

func (c *compositeFile) WriteString(s string) (ret int, err error) {
	return c.file.WriteString(s)
}

type compositeFileInfo struct {
	fileInfo os.FileInfo
	name     string
}

func (c *compositeFileInfo) Name() string {
	return c.name
}

func (c *compositeFileInfo) Size() int64 {
	return c.fileInfo.Size()
}

func (c *compositeFileInfo) Mode() os.FileMode {
	return c.fileInfo.Mode()
}

func (c *compositeFileInfo) ModTime() time.Time {
	return c.fileInfo.ModTime()
}

func (c *compositeFileInfo) IsDir() bool {
	return c.fileInfo.IsDir()
}

func (c *compositeFileInfo) Sys() interface{} {
	return nil
}

type compositeShadowFile struct {
	fs       *CompositeFs
	name     string
	path     string
	children []string
}

func (c *compositeShadowFile) Close() error {
	return syscall.EPERM
}

func (c *compositeShadowFile) Read(p []byte) (n int, err error) {
	return 0, syscall.EPERM
}

func (c *compositeShadowFile) ReadAt(p []byte, off int64) (n int, err error) {
	return 0, syscall.EPERM
}

func (c *compositeShadowFile) Seek(offset int64, whence int) (int64, error) {
	return 0, syscall.EPERM
}

func (c *compositeShadowFile) Write(p []byte) (n int, err error) {
	return 0, syscall.EPERM
}

func (c *compositeShadowFile) WriteAt(p []byte, off int64) (n int, err error) {
	return 0, syscall.EPERM
}

func (c *compositeShadowFile) Name() string {
	return c.path
}

func (c *compositeShadowFile) Readdir(count int) ([]os.FileInfo, error) {
	fileInfos := make([]os.FileInfo, 0)
	for _, child := range c.children {
		c, err := c.fs.Open(child)
		if err != nil {
			return nil, err
		}

		fileInfo, err := c.Stat()
		if err != nil {
			return nil, err
		}
		fileInfos = append(fileInfos, fileInfo)

		if len(fileInfos) == count {
			return fileInfos, nil
		}
	}

	if len(fileInfos) < count {
		return fileInfos, io.EOF
	}

	return fileInfos, nil
}

func (c *compositeShadowFile) Readdirnames(n int) ([]string, error) {
	fileInfos, err := c.Readdir(n)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(fileInfos))
	for i, fi := range fileInfos {
		names[i] = fi.Name()
	}
	return names, nil
}

func (c *compositeShadowFile) Stat() (os.FileInfo, error) {
	return &compositeShadowFileInfo{
		name: c.name,
		time: c.fs.creationTime,
	}, nil
}

func (c *compositeShadowFile) Sync() error {
	return syscall.EPERM
}

func (c *compositeShadowFile) Truncate(size int64) error {
	return syscall.EPERM
}

func (c *compositeShadowFile) WriteString(s string) (ret int, err error) {
	return 0, syscall.EPERM
}

type compositeShadowFileInfo struct {
	name string
	time time.Time
}

func (c *compositeShadowFileInfo) Name() string {
	return c.name
}

func (c *compositeShadowFileInfo) Size() int64 {
	return 0
}

func (c *compositeShadowFileInfo) Mode() os.FileMode {
	return os.ModePerm
}

func (c *compositeShadowFileInfo) ModTime() time.Time {
	return c.time
}

func (c *compositeShadowFileInfo) IsDir() bool {
	return true
}

func (c *compositeShadowFileInfo) Sys() interface{} {
	return nil
}
