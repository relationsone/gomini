package gomini

type moduleOrigin struct {
	filename string
	path     string
}

func (o *moduleOrigin) Filename() string {
	return o.filename
}

func (o *moduleOrigin) Path() string {
	return o.path
}

type moduleBundle struct {

}