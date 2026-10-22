+++
title = "Kustomize imports"
summary = "kustomize+https:// runs kustomize on a remote kustomization and imports the result"
aka = ["kustomize+https://"]
kind = "engine"
intent = "goal"
research = "done"
status = "done"
refs = ["utils/kustomize.go"]

[[needs]]
on = "/imports/universal"
why = "Registered as one of its URL schemes"

[[needs]]
on = "/deps/kustomize"

[[needs]]
on = "/config/feature-gates"
why = "Alpha: the kustomize-import feature"
+++
