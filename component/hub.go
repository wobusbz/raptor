package component

type Components struct {
	Comp map[string]Component
}

func (cs *Components) Register(name string, c Component) {
	cs.Comp[name] = c
}

func (cs *Components) dispatch() {}
