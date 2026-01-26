package conf

import "time"

type Bootstrap struct {
	Server Server `json:"server" yaml:"server"`
	Data   Data   `json:"data" yaml:"data"`
}

type Server struct {
	HTTP HTTP `json:"http" yaml:"http"`
}

type HTTP struct {
	Network string        `json:"network" yaml:"network"`
	Addr    string        `json:"addr" yaml:"addr"`
	Timeout time.Duration `json:"timeout" yaml:"timeout"`
}

type Data struct {
	Database Database `json:"database" yaml:"database"`
	OpenAI   OpenAI   `json:"openai" yaml:"openai"`
}

type Database struct {
	Driver string `json:"driver" yaml:"driver"`
	Source string `json:"source" yaml:"source"`
}

type OpenAI struct {
	BaseURL string        `json:"base_url" yaml:"base_url"`
	APIKey  string        `json:"api_key" yaml:"api_key"`
	Model   string        `json:"model" yaml:"model"`
	Timeout time.Duration `json:"timeout" yaml:"timeout"`
}
