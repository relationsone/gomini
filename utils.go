package gomini

import (
	"os"
	"strings"
	"crypto/sha256"
	"encoding/hex"
	"github.com/spf13/afero"
)

const bannerLarge = `       __           __  _                                _      _       
  ___ / /____ _____/ /_(_)__  ___ _     ___ ____  __ _  (_)__  (_)
 (_-</ __/ _ \/ __/ __/ / _ \/ _ \/    / _ \/ _ \/  ' \/ / _ \/ /   _ _ _ 
/___/\__/\_,_/_/  \__/_/_//_/\_, /     \_, /\___/_/_/_/_/_//_/_/   (_|_|_)
                            /___/     /___/
`

func fileExists(filesystem afero.Fs, filename string) bool {
	if _, err := filesystem.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func isTypeScript(filename string) bool {
	return strings.HasSuffix(filename, ".ts") ||
		strings.HasSuffix(filename, ".d.ts") ||
		strings.HasSuffix(filename, ".ts.gz") ||
		strings.HasSuffix(filename, ".d.ts.gz") ||
		strings.HasSuffix(filename, ".ts.bz2") ||
		strings.HasSuffix(filename, ".d.ts.bz2")
}

func isJavaScript(filename string) bool {
	return strings.HasSuffix(filename, ".js") ||
		strings.HasSuffix(filename, ".js.gz") ||
		strings.HasSuffix(filename, ".js.bz2")
}

func hash(value string) string {
	hasher := sha256.New()
	hasher.Write([]byte(value))
	sum := hasher.Sum(nil)
	return hex.EncodeToString(sum)
}

func loadPlainJavascript(kernel *kernel, filename string, loader, target Bundle) (Value, error) {
	scriptPath := kernel.resolveScriptPath(loader, filename)
	if prog, err := kernel.loadScriptSource(scriptPath, true); err != nil {
		return nil, err
	} else {
		return executeJavascript(prog, target)
	}
}

func tsCacheFilename(path string, bundle Bundle, kernel *kernel) string {
	kernelBasedPath := kernel.toKernelPath(path, bundle)
	return hash(kernelBasedPath)
}

func executeJavascript(prog Script, bundle Bundle) (Value, error) {
	return bundle.Sandbox().Execute(prog)
}
