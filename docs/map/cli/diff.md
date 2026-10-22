+++
title = "diff"
summary = "Show how the config differs from what is running in the cluster"
aka = ["kubecfg diff"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/diff.go", "pkg/kubecfg/diff.go", "docs/Basic-Usage/Kubecfg-Commands/diff.md"]

[[feature]]
id = "strategies"
title = "--diff-strategy: all, subset or last-applied"
status = "done"

[[feature]]
id = "omit-secrets"
title = "--omit-secrets: hide the values of Secrets"
status = "done"

[[needs]]
on = "/engine/vm"
why = "Evaluates the input files"

[[needs]]
on = "/engine/objects"
why = "Turns the result into a flat list of objects"

[[needs]]
on = "/cluster/client"
why = "Reads the live objects"
+++
