package api

type GlobalEnvListItem struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Target string `json:"target"`
	Secret bool   `json:"secret"`
}

type GlobalEnvResponse struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Target string `json:"target"`
	Secret bool   `json:"secret"`
}

type GlobalEnvPutRequest struct {
	Value  string `json:"value"`
	Target string `json:"target"`
	Secret *bool  `json:"secret,omitempty"`
}
