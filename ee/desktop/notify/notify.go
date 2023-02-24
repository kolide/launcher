package notify

type Notification struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	ActionUri string `json:"action_uri,omitempty"`
}
