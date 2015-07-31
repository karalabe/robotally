// Package robotally is an AppEngine based GitHub webhook to aggregate review
// votes on opened pull requests.
package robotally

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	"google.golang.org/appengine"
)

// Disabled emojis to not count certain common reactions.
var disabled = map[string]bool{":+1": true, ":-1": true}

// Pass all requests through a single handler
func init() {
	http.HandleFunc("/", handler)
}

// handler is the global HTTP request handler processing the GitHub webhook events.
func handler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	// Read the entire request body
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	// Decode any GitHub event, and check for outside "opened" or "created" actions exclusively
	e := new(Event)
	if err := json.Unmarshal(body, e); err != nil {
		http.Error(w, "Invalid GitHub event", http.StatusBadRequest)
		return
	}
	if e.Sender.Login == githubUser {
		return
	}
	if e.Action != "opened" && e.Action != "created" {
		http.Error(w, "Non-supported action", http.StatusMethodNotAllowed)
		return
	}
	// Create an authenticated GitHub client
	auth := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	))
	client := github.NewClient(auth)

	// Handle the event, depending whether creation or comment
	switch e.Action {
	case "opened":
		// A new issue or pull request was opened, add an empty status report to it
		number := 0
		if e.Issue != nil {
			number = e.Issue.Number
		} else if e.PullRequest != nil {
			number = e.PullRequest.Number
		}
		// Make sure to also issue a warning if against master
		warning := ""
		if e.PullRequest != nil && e.PullRequest.Base.Branch == "master" {
			warning = "Pull request against `master`"
		}
		report := status(warning, nil, nil)
		if _, _, err := client.Issues.CreateComment(e.Repository.Owner.Login, e.Repository.Name, number, &github.IssueComment{Body: &report}); err != nil {
			http.Error(w, fmt.Sprintf("Failed to comment on issue: %v", err), http.StatusInternalServerError)
			return
		}

	case "created":
		// A comment was added, gather all reactions
		comments, _, err := client.Issues.ListComments(e.Repository.Owner.Login, e.Repository.Name, e.Issue.Number, &github.IssueListCommentsOptions{Sort: "created"})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to list comments: %v", err), http.StatusInternalServerError)
			return
		}
		// Aggregate the votes from every comment and retain any warning messages
		votes, reactions := aggregate(comments)

		warning := ""
		for _, comment := range comments {
			if *comment.User.Login == githubUser {
				if matches := regexp.MustCompile(":exclamation: (.*) :exclamation:").FindAllStringSubmatch(comment.String(), -1); len(matches) > 0 {
					warning = matches[0][1]
				}
			}
		}
		// Generate a fresh status report and edit the old one
		report := status(warning, votes, reactions)
		for _, comment := range comments {
			if *comment.User.Login == githubUser {
				if _, _, err := client.Issues.EditComment(e.Repository.Owner.Login, e.Repository.Name, *comment.ID, &github.IssueComment{Body: &report}); err != nil {
					http.Error(w, fmt.Sprintf("Failed to update issue report: %v", err), http.StatusInternalServerError)
					return
				}
				return
			}
		}
	}
}

// aggregate iterates over all the comments of a PR and aggregates the review
// votes and any other allowed emoji reactions.
func aggregate(comments []github.IssueComment) (map[string]bool, map[string]map[string]struct{}) {
	votes := make(map[string]bool)
	reactions := make(map[string]map[string]struct{})

	// Iterate all the comments and extract the reactions
	for _, comment := range comments {
		// Short circuit if our own comment
		if *comment.User.Login == githubUser {
			continue
		}
		// Scan through the comment and find and up or down votes
		if strings.Contains(comment.String(), ":+1:") {
			votes[*comment.User.Login] = true
		} else if strings.Contains(comment.String(), ":-1:") {
			votes[*comment.User.Login] = false
		}
		// Find all other emojis withn the comment
		emojis := regexp.MustCompile(":[a-z0-9_]+:")
		for _, emoji := range emojis.FindAllString(comment.String(), -1) {
			if !disabled[emoji] {
				// Make sure we have a valid user set
				if _, ok := reactions[emoji]; !ok {
					reactions[emoji] = make(map[string]struct{})
				}
				reactions[emoji][*comment.User.Login] = struct{}{}
			}
		}
	}
	return votes, reactions
}

// status renders a new status report based on the PR votes as well as any
// additional allowed emojis.
func status(warning string, votes map[string]bool, emojis map[string]map[string]struct{}) string {
	report := ""

	// Issues any warning if requested
	if len(warning) > 0 {
		report += ":exclamation: " + warning + " :exclamation:\n\n"
	}
	// Collect the number of upvotes and downvotes
	up, down := []string{}, []string{}
	for user, yes := range votes {
		if yes {
			up = append(up, "@"+user)
		} else {
			down = append(down, "@"+user)
		}
	}
	// Sort the users and generate the review statistics
	sort.Strings(up)
	sort.Strings(down)

	report += fmt.Sprintf("| Vote | Count | Reviewers |\n| :---: | :---: | :---: |\n| :+1: | %d | %s |\n| :-1: | %d | %s |",
		len(up), strings.Join(up, " "), len(down), strings.Join(down, " "))

	// If there were additionally requested emojis, report on them too
	if len(emojis) > 0 {
		// Gather the reactions and assotiated users
		reactions := make(map[string][]string)
		for emoji, users := range emojis {
			for user := range users {
				reactions[emoji] = append(reactions[emoji], "@"+user)
			}
			sort.Strings(reactions[emoji])
		}
		// Order the reactions by frequency
		emojis := make([]string, 0, len(reactions))
		for emoji := range reactions {
			emojis = append(emojis, emoji)
		}
		for i, first := range emojis {
			for j, second := range emojis[i+1:] {
				if len(reactions[first]) < len(reactions[second]) {
					emojis[i], emojis[j] = emojis[j], emojis[i]
					first, second = second, first
				}
			}
		}
		// Generate a report for the reactions too
		report += fmt.Sprintf("\n\n| Reaction | Users |\n| :---: | :---: |\n")
		for _, emoji := range emojis {
			report += fmt.Sprintf("| %s | %s |\n", emoji, strings.Join(reactions[emoji], " "))
		}
	}
	// Add the modification time and return
	return report + fmt.Sprintf("\n\n_Updated: %s_", time.Now().UTC().Format("Mon Jan 2 15:04:05 MST 2006"))
}
