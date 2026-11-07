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

[[needs]]
on = "/config/flags"
+++

Since #538, `t8y:/config/feature-gates` decides each feature on its own;
`--alpha` still turns on every alpha feature that `--features` doesn't name.
