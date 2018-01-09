package gomini

type bundleImpl struct {
	name    string
	baseDir string
}

func newBundle(id, name, baseDir string) (*bundleImpl, error) {
	return &bundleImpl{
		name: name,
		baseDir: baseDir,
	}, nil
}

func (bundle *bundleImpl) Path() string {
	return bundle.baseDir
}