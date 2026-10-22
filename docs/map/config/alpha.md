+++
title = "--alpha"
summary = "--alpha or KUBECFG_ALPHA turns on every alpha feature at once"
aka = ["KUBECFG_ALPHA", "alpha"]
kind = "config"
intent = "induced"
research = "done"
refs = ["cmd/root.go"]

[[feature]]
id = "switch"
title = "--alpha and KUBECFG_ALPHA"
status = "done"

[[feature]]
id = "jsonnet"
title = "std.native('alpha') and kubecfg.alpha: Jsonnet code can see the switch"
status = "wip"
note = "On the worktree-abstract-snuggling-pixel branch, with knit"

[[needs]]
on = "/config/flags"
+++

Since #538, `t8y:/config/feature-gates` decides each feature on its own;
`--alpha` still turns on every alpha feature that `--features` doesn't name.
