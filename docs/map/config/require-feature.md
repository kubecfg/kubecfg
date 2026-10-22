+++
title = "requireFeature"
summary = "kubecfg.requireFeature('knit'): a library fails clearly when a feature it needs is off"
kind = "config"
intent = "aspiration"
research = "outline"
status = "todo"

[[needs]]
on = "/config/feature-gates"

[[needs]]
on = "/lang/stdlib"
why = "Where the function would live"
+++

An open question in RFC05. Requiring a feature seems fine; reading one
(`if enabled then ... else ...`) would make output depend on flags.
