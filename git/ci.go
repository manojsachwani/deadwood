package git

import (
	"encoding/json"
	"os/exec"
)

// CI results as shown next to a branch
const (
	CIPass    = "pass"
	CIFail    = "fail"
	CIRunning = "running"
)

// ciByCommit asks the GitHub CLI for recent workflow runs and returns one
// combined result per commit: fail if any workflow failed, running if any is
// still going, pass if all succeeded. Only the newest run of each workflow
// counts, so a re-run replaces the attempt before it. It returns nil when gh
// is missing, not logged in, or the repository is not on GitHub.
func ciByCommit() map[string]string {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil
	}
	out, err := exec.Command("gh", "run", "list", "--limit", "200",
		"--json", "headSha,workflowName,status,conclusion").Output()
	if err != nil {
		return nil
	}
	var runs []struct {
		HeadSha      string `json:"headSha"`
		WorkflowName string `json:"workflowName"`
		Status       string `json:"status"`
		Conclusion   string `json:"conclusion"`
	}
	if json.Unmarshal(out, &runs) != nil {
		return nil
	}

	// gh lists runs newest first.
	seen := map[[2]string]bool{}
	results := map[string]string{}
	for _, r := range runs {
		key := [2]string{r.HeadSha, r.WorkflowName}
		if seen[key] {
			continue
		}
		seen[key] = true

		result := CIPass
		switch {
		case r.Status != "completed":
			result = CIRunning
		case r.Conclusion == "success" || r.Conclusion == "skipped" || r.Conclusion == "neutral":
		default:
			result = CIFail
		}
		switch current := results[r.HeadSha]; {
		case current == CIFail || result == CIFail:
			results[r.HeadSha] = CIFail
		case current == CIRunning || result == CIRunning:
			results[r.HeadSha] = CIRunning
		default:
			results[r.HeadSha] = CIPass
		}
	}
	return results
}

// applyCI fills in Branch.CI for branches whose tip commit has workflow runs.
// A branch with local commits that were never pushed has no result.
func applyCI(branches []Branch) {
	results := ciByCommit()
	for i := range branches {
		branches[i].CI = results[branches[i].SHA]
	}
}
