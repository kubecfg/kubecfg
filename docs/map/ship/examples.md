+++
title = "Examples"
summary = "Example configs, tested so we know when a change breaks them"
aka = ["guestbook", "golden files"]
kind = "infra"
intent = "goal"
research = "done"
refs = ["examples/", "pkg/kubecfg/examples_test.go"]

[[feature]]
id = "guestbook"
title = "The guestbook, in Jsonnet and in YAKL, with an overlay"
status = "done"

[[feature]]
id = "golden"
title = "Every example checked against golden files"
status = "wip"
note = "Only the k8s-libsonnet examples are checked on main; the knit branch checks them all"

[[feature]]
id = "kustomize-yakl"
title = "kustomize with YAKL expressions: typed variables for kustomize"
status = "wip"
note = "examples/kustomize-jamlet, uncommitted in the structured-stargazing-beacon worktree"
needs = ["/lang/yakl#kubecfg-locals"]
+++
