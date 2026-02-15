package conf

type Bootstrap struct {
	Server    Server    `json:"server" yaml:"server"`
	Upstreams Upstreams `json:"upstreams" yaml:"upstreams"`
}

type Server struct {
	HTTP HTTP `json:"http" yaml:"http"`
}

type HTTP struct {
	Network string `json:"network" yaml:"network"`
	Addr    string `json:"addr" yaml:"addr"`
	Timeout string `json:"timeout" yaml:"timeout"`
}

type Upstreams struct {
	User  Upstream `json:"user" yaml:"user"`
	Goods Upstream `json:"goods" yaml:"goods"`
	Order Upstream `json:"order" yaml:"order"`
}

type Upstream struct {
	BaseURL string `json:"base_url" yaml:"base_url"`
	Timeout string `json:"timeout" yaml:"timeout"`
}
