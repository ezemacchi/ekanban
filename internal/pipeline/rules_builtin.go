package pipeline

// The built-in rules, one type each. DefaultRules runs them in this order.

func init() {
	Register(deployedRule{})
	Register(mergedRule{})
	Register(notStartedRule{})
	Register(workingRule{})
	Register(landedRule{})
}

// deployedRule: merged and included in the target's last publish.
type deployedRule struct{}

func (deployedRule) Name() string { return "deployed" }
func (deployedRule) Decide(f Facts) (Decision, bool) {
	return Decision{Stage: ReadyQA}, f.MergedTo != "" && f.Deployed != ""
}

// mergedRule: merged but not published yet.
type mergedRule struct{}

func (mergedRule) Name() string { return "merged" }
func (mergedRule) Decide(f Facts) (Decision, bool) {
	return Decision{Stage: ToDeploy, Note: f.Offline}, f.MergedTo != ""
}

// notStartedRule: no run, or a run with nothing dispatched.
type notStartedRule struct{}

func (notStartedRule) Name() string { return "not-started" }
func (notStartedRule) Decide(f Facts) (Decision, bool) {
	return Decision{Stage: ToDo}, !f.HasRun || f.NotStarted
}

// workingRule: the team is on it and has not landed.
type workingRule struct{}

func (workingRule) Name() string { return "working" }
func (workingRule) Decide(f Facts) (Decision, bool) {
	return Decision{Stage: InProgress}, !f.Landed
}

// landedRule: landed, so the pull request is with the reviewers.
type landedRule struct{}

func (landedRule) Name() string { return "landed" }
func (landedRule) Decide(f Facts) (Decision, bool) {
	d := Decision{Stage: OnReview}
	if f.PR == 0 {
		d.Note = f.Offline
	}
	return d, f.Landed
}
