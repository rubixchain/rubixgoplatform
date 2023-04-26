package model

// BasicResponse will be basic response model
type BasicResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Result  interface{} `json:"result"`
}

// TokenNumberResponse will be basic response model
type TokenNumberResponse struct {
	Status       bool   `json:"status"`
	Message      string `json:"message"`
	TokenNumbers []int  `json:"tokennumbers"`
}

// MigratedToken Check
type MigratedTokenStatus struct {
	Status         bool   `json:"status"`
	Message        string `json:"message"`
	MigratedStatus []int  `json:"migratedstatus"`
}
