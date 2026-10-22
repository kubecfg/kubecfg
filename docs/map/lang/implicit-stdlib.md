+++
title = "Implicit kubecfg"
summary = "kubecfg is a predefined name, so files use the library without importing it"
aka = ["implicit-stdlib"]
kind = "lang"
intent = "aspiration"
research = "outline"
status = "todo"

[[needs]]
on = "/lang/stdlib"

[[needs]]
on = "/config/feature-gates"
why = "Starts as the alpha feature implicit-stdlib"

[[needs]]
fuzzy = "go-jsonnet: let the embedding program add globals, the way std is added"
why = "Prepending text to each file would change what importstr returns"
+++

A follow-up in RFC05. It is additive: a free `kubecfg` variable is an error
today, so no working file depends on it being missing.
