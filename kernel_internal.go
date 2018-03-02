package gomini

import (
	"github.com/satori/go.uuid"
	"github.com/apex/log"
	"github.com/go-errors/errors"
	"path/filepath"
)

func (k *kernel) __resolveDependencyModule(dependency string, bundle *bundle, module *module) (Module, error) {
	scriptPath := k.resolveScriptPath(bundle, dependency)

	vfs, file, err := k.__toVirtualKernelFile(scriptPath)
	if err != nil {
		return nil, err
	}

	if vfs {
		kernelModule := file.syscall(bundle).(Module)
		log.Debugf("Kernel: Resolved dependency %s [virtual module file to '%s:/%s']",
			dependency, kernelModule.Bundle().Name(), kernelModule.Origin().FullPath())

		log.Debugf("Kernel: Needs security proxy for exported modules '%s:/%s' to '%s:/%s'",
			bundle.Name(), module.Origin().FullPath(), kernelModule.Bundle().Name(), kernelModule.Origin().FullPath())

		if err == nil {
			// We panic if access not granted
			if err := kernelModule.IsAccessible(bundle); err != nil {
				panic(err)
			}
			return kernelModule, nil
		}
	}

	dependentModule := bundle.findModuleByModuleFile(scriptPath.path)
	if dependentModule == nil {
		id, err := uuid.NewV4()
		if err != nil {
			return nil, err
		}

		log.Debugf("Kernel: Resolved dependency %s [%s:/%s]*", dependency, scriptPath.loader.Name(), scriptPath.path)

		moduleId := id.String()
		m, err := k.loadScriptModule(moduleId, dependency, module.origin.Path(), scriptPath, bundle)
		if err != nil {
			panic(err)
		}

		return m, nil
	}

	log.Debugf("Kernel: Reused already loaded module %s (%s:/%s) with id %s",
		dependency, dependentModule.Bundle().Name(), scriptPath.path, dependentModule.ID())

	log.Debugf("Kernel: Resolved dependency %s [%s:/%s]", dependency, scriptPath.loader.Name(), scriptPath.path)

	return dependentModule, nil
}

func (k *kernel) __loadSource(bundle Bundle, filename string) (string, error) {
	if isTypeScript(filename) {
		// Is pre-transpiled?
		cacheFilename := filepath.Join(KernelVfsCachePath, tsCacheFilename(filename, bundle, k))
		if !fileExists(k.Filesystem(), cacheFilename) {
			log.Debugf("Kernel: Loading scriptfile '%s:/%s' with live transpiler", bundle.Name(), filename)

			source, err := k.__transpile(bundle, filename)
			if err != nil {
				return "", err
			}

			// DEBUG
			if source != nil {
				log.Debug(*source)
			}
			return *source, nil
		}

		// Override filename with the pre-transpiled, cached file
		log.Debugf("Kernel: Loading scriptfile '%s:/%s' from pretranspiled cache: kernel:/%s",
			bundle.Name(), filename, cacheFilename)

		if data, err := k.loadContent(bundle, k.Filesystem(), cacheFilename); err != nil {
			return "", err
		} else {
			return string(data), nil
		}
	}
	if data, err := k.loadContent(bundle, bundle.Filesystem(), filename); err != nil {
		return "", err
	} else {
		return string(data), nil
	}
}

func (k *kernel) __transpile(bundle Bundle, filename string) (*string, error) {
	transpiler, err := newTranspiler(k)
	if err != nil {
		return nil, errors.New(err)
	}
	return transpiler.transpileFile(bundle, filename)
}

func (k *kernel) __toVirtualKernelFile(scriptPath *resolvedScriptPath) (bool, *kernelFile, error) {
	bundle := scriptPath.loader
	f, err := bundle.Filesystem().Open(scriptPath.path)
	if err != nil {
		return false, nil, err
	}
	switch ff := f.(type) {
	case *compositeFile:
		e, success := ff.file.(*kernelFile)
		return success && !e.dir, e, nil
	}
	return false, nil, nil
}
