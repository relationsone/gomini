package gomini

type Origin interface {
	Filename() string
	Path() string
	FullPath() string
}

type Module interface {
	ID() string
	Name() string
	Origin() Origin
	Bundle() Bundle

	IsAccessible(caller Bundle) error

	export(value Value, target interface{}) error
	getModuleExports() Object
	setName(name string)
}
