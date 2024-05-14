package model

// ExplorerLinks
type ExplorerLinks struct {
	Links []string `json:"links"`
}

// ExplorerResponse used as model for the API responses
type ExplorerResponse struct {
	Status  bool          `json:"status"`
	Message string        `json:"message"`
	Result  ExplorerLinks `json:"result"`
}
