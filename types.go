package robotally

// Event is the GitHub webhook notification of a repository action.
type Event struct {
	Action      string       `json:"action"`
	Issue       *Issue       `json:"issue"`
	PullRequest *PullRequest `json:"pull_request"`
	Repository  *Repository  `json:"repository"`
	Sender      *User        `json:"sender"`
}

// Issue represents the data about the issue being reported on.
type Issue struct {
	Number int `json:"number"`
}

// PullRequest represents the data about the PR being reported on.
type PullRequest struct {
	Number int       `json:"number"`
	Base   *Endpoint `json:"base"`
}

// Repository represents the repository originating a webhook event.
type Repository struct {
	Name  string `json:"name"`
	Owner *User  `json:"owner"`
}

// Endpoint represents one of the enpoints of a PR comparison.
type Endpoint struct {
	Branch string `json:"ref"`
}

// User represents a GitHub user.
type User struct {
	Login string `json:"login"`
}
