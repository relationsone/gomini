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
		log.Debugf("Kernel: Resolved dependency %s [virtual module file to %s:/%s]",
			dependency, file.module.Bundle().Name(), file.module.Origin().FullPath())

		log.Infof("Kernel: Needs kernel intervention to get exported modules from %s:/%s to %s:/%s",
			file.module.Bundle().Name(), file.module.Origin().FullPath(), bundle.Name(), module.Origin().FullPath())

		if err == nil {
			property := file.module.Name() + ".inject"
			sandboxSecurityCheck(property, file.module.Bundle(), bundle)
			return file.module, nil
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
		cacheFilename := filepath.Join(cacheVfsPath, tsCacheFilename(filename, bundle, k))
		if !fileExists(k.Filesystem(), cacheFilename) {
			log.Infof("Kernel: Loading scriptfile '%s:/%s' with live transpiler", bundle.Name(), filename)

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
		log.Infof("Kernel: Loading scriptfile '%s:/%s' from pretranspiled cache: kernel:/%s",
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

func (k *kernel) __toVirtualKernelFile(scriptPath *resolvedScriptPath) (bool, *exportFile, error) {
	bundle := scriptPath.loader
	f, err := bundle.Filesystem().Open(scriptPath.path)
	if err != nil {
		return false, nil, err
	}
	switch ff := f.(type) {
	case *compositeFile:
		e, success := ff.file.(*exportFile)
		return success && !e.dir, e, nil
	}
	return false, nil, nil
}
