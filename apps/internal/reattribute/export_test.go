package reattribute

// OnTables points the retrier at other tables shaped like
// attribution_retries and event_queue (testpg.PrivateTable).
func (r *Retrier) OnTables(retries, queue string) *Retrier {
	r.retries, r.queue = retries, queue
	return r
}
