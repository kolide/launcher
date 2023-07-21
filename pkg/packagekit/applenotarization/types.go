package applenotarization

import "time"

type notarizationResponse struct {
	Message string `json:"message"`
	ID      string `json:"id"`
	Path    string `json:"path"`
}

type notarizationInfoResponse struct {
	Message     string    `json:"message"`
	ID          string    `json:"id"`
	CreatedDate time.Time `json:"createdDate"`
	Status      string    `json:"status"`
	Name        string    `json:"name"`
}
