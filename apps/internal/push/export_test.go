package push

// ScopedTo is the repository whose worker sends (SendDue, SendTriggered)
// touch only these organizations — see Repository.scope.
func (r *Repository) ScopedTo(orgIDs ...string) *Repository {
	c := *r
	c.scope = orgIDs
	return &c
}
