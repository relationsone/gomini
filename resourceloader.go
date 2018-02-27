package gomini

import (
	"os"
	"io/ioutil"
	"github.com/spf13/afero"
)

type resourceLoader struct {
}

func newResourceLoader() ResourceLoader {
	return &resourceLoader{}
}

func (rl *resourceLoader) LoadResource(kernel *kernel, filesystem afero.Fs, filename string) ([]byte, error) {
	file, err := filesystem.OpenFile(filename, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ioutil.ReadAll(file)
}
