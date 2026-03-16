package models

type EventTransaction struct {
	Transaction *Transactions `json:"transaction"`
	Status      bool         `json:"status"`
	Message     string       `json:"message"`
}
