package git

import (
	"encoding/json"
	"os/exec"
)

// PRMerged marks a branch whose pull request was merged on GitHub
const PRMerged MergeKind = "pr-merged"

// mergedPullRequests asks the GitHub CLI for recently merged pull requests
// and returns head branch name -> head commit. It returns nil when gh is
// missing, not logged in, or the repository is not on GitHub.
func mergedPullRequests() map[string]string {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil
	}
	out, err := exec.Command("gh", "pr", "list", "--state", "merged", "--limit", "300",
		"--json", "headRefName,headRefOid").Output()
	if err != nil {
		return nil
	}
	var prs []struct {
		HeadRefName string `json:"headRefName"`
		HeadRefOid  string `json:"headRefOid"`
	}
	if json.Unmarshal(out, &prs) != nil {
		return nil
	}
	heads := make(map[string]string, len(prs))
	for _, pr := range prs {
		heads[pr.HeadRefName] = pr.HeadRefOid
	}
	return heads
}

// applyPullRequests marks branches that patch comparison could not prove
// merged but whose exact tip was merged through a pull request. A branch
// with commits added after the merge keeps its unmerged state.
func applyPullRequests(branches []Branch) {
	heads := mergedPullRequests()
	for i := range branches {
		b := &branches[i]
		if b.Merge == NotMerged && !b.Protected && heads[b.Name] == b.SHA && b.SHA != "" {
			b.Merge = PRMerged
		}
	}
}
