package controller

type Version struct {
	Data     string `json:"data"`
	Created  string `json:"created"`
	Modified string `json:"modified"`
}

type Secret struct {
	Versions map[string]Version `json:"versions"`
}

type Namespace struct {
	Secrets map[string]Secret `json:"secrets"`
}

type ConfigData struct {
	Namespaces map[string]Namespace `json:"namespaces"`
}
