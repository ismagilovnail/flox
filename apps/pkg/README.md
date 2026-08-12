# pkg

Code safe for import outside this module (e.g. by `apps/tracker`/`apps/worker`
once they exist, or a future generated API client) — as opposed to
`internal/`, which Go itself prevents anyone outside this module from
importing. Empty until something actually needs to be shared that way;
nothing does yet in Phase 16.
