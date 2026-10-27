+++
title = "requireFeature"
summary = "kubecfg.requireFeature('knit'): a library fails clearly when a feature it needs is off"
kind = "config"
intent = "induced"
research = "outline"
status = "done"
refs = ["lib/kubecfg.libsonnet", "utils/nativefuncs.go"]

[[needs]]
on = "/config/feature-gates"

[[needs]]
on = "/lang/stdlib"
why = "Where the function would live"
+++

An open question in RFC05. Requiring a feature seems fine; reading one
(`if enabled then ... else ...`) would make output depend on flags, so
there is no function for that. knit is its first user.
