+++
title = "delete"
summary = "Delete every object in the config, in reverse dependency order"
aka = ["kubecfg delete"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/delete.go", "pkg/kubecfg/delete.go", "docs/Basic-Usage/Kubecfg-Commands/delete.md"]

[[feature]]
id = "delete"
title = "Delete the objects, last-needed first"
status = "done"

[[feature]]
id = "grace-period"
title = "--grace-period for terminating objects"
status = "done"

[[needs]]
on = "/engine/vm"
why = "Evaluates the input files"

[[needs]]
on = "/engine/objects"
why = "Turns the result into a flat list of objects"

[[needs]]
on = "/cluster/client"
why = "Deletes the objects"

[[needs]]
on = "/cluster/ordering"
why = "Deletes in the reverse of the apply order"
+++
