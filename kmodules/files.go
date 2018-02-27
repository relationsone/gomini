package kmodules

import (
	"github.com/relationsone/gomini"
	"reflect"
	"os"
	"path/filepath"
	"github.com/spf13/afero"
)

type files string

const kmoduleFiles files = "0c97cffa-f27f-49f9-95cf-5472a98253a9"

func NewFilesModule() gomini.KernelModule {
	return kmoduleFiles
}

func (f files) ID() string {
	return string(f)
}

func (files) Name() string {
	return "files"
}

func (files) ApiDefinitionFile() string {
	return "/kernel/@types/files"
}

func (files) SecurityInterceptor() gomini.SecurityInterceptor {
	return func(caller gomini.Bundle, property string) bool {
		// Default checks are enough
		return true
	}
}

func (files) KernelModuleBinder() gomini.KernelModuleBinder {
	return func(bundle gomini.Bundle, builder gomini.JsObjectBuilder) {
		resolve := func(ppath string) (*path, error) {
			info, err := bundle.Filesystem().Stat(ppath)
			if !os.IsNotExist(err) {
				return nil, err
			}

			filetype := ft_unknown
			if info != nil {
				if info.IsDir() {
					filetype = ft_directory
				} else if gomini.IsKernelFile(bundle.Filesystem(), ppath) {
					filetype = ft_kernel
				} else {
					filetype = ft_file
				}
			}

			return &path{
				name:     filepath.Base(ppath),
				path:     ppath,
				bundle:   bundle,
				filetype: filetype,
			}, nil
		}

		builder.DefineFunction("resolvePath", func(call gomini.JsFunctionCall) gomini.JsValue {
			if len(call.Arguments) < 1 {
				return bundle.NewTypeError("illegal number of arguments")
			}

			val := call.Argument(0)
			if val.ExportType().Kind() != reflect.String {
				return bundle.NewTypeError("illegal parameter type")
			}

			p := val.String()
			path, err := resolve(p)
			if err != nil {
				return bundle.NewTypeError(err)
			}

			return path.adapt(resolve)
		})
	}
}

type filetype int8

const (
	ft_unknown   filetype = 1
	ft_kernel    filetype = 2
	ft_directory filetype = 4
	ft_file      filetype = 8
)

type path struct {
	name     string
	path     string
	filetype filetype
	bundle   gomini.Bundle
}

func (p *path) exists(call gomini.JsFunctionCall) gomini.JsValue {
	exists, err := afero.Exists(p.bundle.Filesystem(), p.path)
	if err != nil {
		return p.bundle.NewTypeError(err)
	}
	return p.bundle.ToValue(exists)
}

func (p *path) mkdir(call gomini.JsFunctionCall) gomini.JsValue {
	if len(call.Arguments) < 1 {
		return p.bundle.NewTypeError("illegal number of arguments")
	}

	val := call.Argument(0)
	if val.ExportType().Kind() != reflect.Bool {
		return p.bundle.NewTypeError("illegal parameter type")
	}

	var err error
	if val.ToBoolean() {
		err = p.bundle.Filesystem().MkdirAll(p.path, os.ModePerm)
	} else {
		err = p.bundle.Filesystem().Mkdir(p.path, os.ModePerm)
	}

	if err != nil {
		return p.bundle.NewTypeError(err)
	}

	return p.bundle.Undefined()
}

func (p *path) resolve(resolve func(string) (*path, error)) func(gomini.JsFunctionCall) gomini.JsValue {
	return func(call gomini.JsFunctionCall) gomini.JsValue {
		if len(call.Arguments) < 1 {
			return p.bundle.NewTypeError("illegal number of arguments")
		}

		val := call.Argument(0)
		if val.ExportType().Kind() != reflect.String {
			return p.bundle.NewTypeError("illegal parameter type")
		}

		ppath := val.String()
		path, err := resolve(filepath.Join(p.path, ppath))
		if err != nil {
			return p.bundle.NewTypeError(err)
		}

		return path.adapt(resolve)
	}
}

func (p *path) toFile(call gomini.JsFunctionCall) gomini.JsValue {
	// TODO
	return p.bundle.Undefined()
}

func (p *path) toPipe(call gomini.JsFunctionCall) gomini.JsValue {
	// TODO
	return p.bundle.Undefined()
}

func (p *path) adapt(resolve func(string) (*path, error)) gomini.JsObject {
	builder := p.bundle.NewObjectBuilder("path")
	builder.
		DefineConstant("name", p.name).
		DefineConstant("path", p.path).
		DefineConstant("type", p.filetype).
		DefineFunction("exists", p.exists).
		DefineFunction("mkdir", p.mkdir).
		DefineFunction("resolve", p.resolve(resolve)).
		DefineFunction("toFile", p.toFile).
		DefineFunction("toPipe", p.toPipe)
	return builder.Build()
}
