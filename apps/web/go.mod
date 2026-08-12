// apps/web contains no Go code — it is the Next.js frontend.
//
// This file exists for exactly one reason: the Go module root moved to
// `apps/` in Phase 21 (so apps/api, apps/tracker and apps/worker can share
// `apps/internal/...`, per ARCHITECTURE.md's "separate binaries inside the
// same Go module"). That puts apps/web inside the module's directory tree,
// which means `go build ./...` would otherwise compile, vet, and test any
// stray .go file shipped inside `web/node_modules/` by an npm package —
// letting a third-party JS dependency break our Go build.
//
// Declaring a nested module here makes the Go toolchain skip this subtree
// entirely. Nothing ever imports it, and it does not affect npm/Next.js.
module github.com/ismagilovnail/flox/apps/web

go 1.26
