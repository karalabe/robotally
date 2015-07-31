package robotally

// Configure the GitHub credentials
const (
	githubUser  = "robotally" // User to aggregate the reviews with
	githubToken = ""          // User's auth token to access the GitHub APIs
)

// Allowed GitHub secrets for preventing rogue requests (empty = allow all).
var githubSecrets = map[string][]byte{}
