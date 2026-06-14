package packages

type Handler interface {
	Type() string
	Install(pkg string) error
	Remove(pkg string) error
}

var handlers = map[string]Handler{}

func Register(h Handler) {
	handlers[h.Type()] = h
}

func Get(typ string) Handler {
	return handlers[typ]
}

func Types() []string {
	types := make([]string, 0, len(handlers))
	for t := range handlers {
		types = append(types, t)
	}
	return types
}
