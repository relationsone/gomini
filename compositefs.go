package gomini

import (
	"github.com/spf13/afero"
	"os"
	"time"
	"path/filepath"
	"strings"
	"errors"
)

const pathSeparator = "/"

type CompositeFs struct {
	base    afero.Fs
	mounts  map[string]afero.Fs
	parents map[string][]string
}

func NewCompositeFs(base afero.Fs) *CompositeFs {
	return &CompositeFs{
		base:    base,
		mounts:  make(map[string]afero.Fs),
		parents: make(map[string][]string),
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
	mount, innerPath := c.findMount(name)
	return mount.Open(innerPath)
}

func (c *CompositeFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	mount, innerPath := c.findMount(name)
	return mount.OpenFile(innerPath, flag, perm)
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
	mount, innerPath := c.findMount(name)
	return mount.Stat(innerPath)
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
