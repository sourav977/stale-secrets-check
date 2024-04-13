package controller

type History struct {
	Data string `json:"data"`
}

type Secret struct {
	Name         string    `json:"name"`
	Created      string    `json:"created"`
	LastModified string    `json:"last_modified"`
	History      []History `json:"history"`
}

type Namespace struct {
	Name    string   `json:"name"`
	Secrets []Secret `json:"secrets"`
}

type ConfigData struct {
	Namespaces []Namespace `json:"namespaces"`
}
