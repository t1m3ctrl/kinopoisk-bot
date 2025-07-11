package model

type SearchState struct {
	Type     string `json:"type"`
	Query    string `json:"query"`
	PersonID int    `json:"person_id"`
	Page     int    `json:"page"`
}
