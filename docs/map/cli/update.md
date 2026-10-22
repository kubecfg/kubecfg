+++
title = "update"
summary = "Create or patch every object in the config, in dependency order, and garbage collect"
aka = ["kubecfg update", "apply"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/update.go", "pkg/kubecfg/update.go", "docs/Basic-Usage/Kubecfg-Commands/update.md"]

[[feature]]
id = "apply"
title = "Create missing objects and patch existing ones"
status = "done"

[[feature]]
id = "create"
title = "--create=false: only patch objects that already exist"
status = "done"

[[feature]]
id = "dry-run"
title = "--dry-run: only read from the cluster"
status = "done"

[[feature]]
id = "validate"
title = "--validate: check objects against the server's schema first"
status = "done"
needs = ["/cluster/openapi"]

[[feature]]
id = "gc"
title = "--gc-tag: delete tagged objects that are no longer in the config"
status = "done"
needs = ["/cluster/gc"]

[[needs]]
on = "/engine/vm"
why = "Evaluates the input files"

[[needs]]
on = "/engine/objects"
why = "Turns the result into a flat list of objects"

[[needs]]
on = "/cluster/client"
why = "Creates and patches the objects"

[[needs]]
on = "/cluster/ordering"
why = "Namespaces and CRDs go in before the objects that use them"
+++
