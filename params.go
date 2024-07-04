package jonson

// paramsSafeguard defines objects that may be used as value containers
type paramsSafeguard interface {
	_isParams()
}

// Params must be embedded as first element in all value containers
type Params struct {
}

func (p *Params) _isParams() {}
