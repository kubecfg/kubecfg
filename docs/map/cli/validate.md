+++
title = "validate"
summary = "Check the rendered objects against the cluster's schema"
aka = ["kubecfg validate"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/validate.go", "pkg/kubecfg/validate.go", "docs/Basic-Usage/Kubecfg-Commands/validate.md"]

[[feature]]
id = "schema"
title = "Validate against the server's OpenAPI schema"
status = "done"

[[feature]]
id = "ignore-unknown"
title = "--ignore-unknown: skip kinds the server has no schema for"
status = "done"

[[feature]]
id = "repeat-eval"
title = "--repeat-eval: evaluate twice and check the output is the same"
status = "done"

[[needs]]
on = "/engine/vm"
why = "Evaluates the input files"

[[needs]]
on = "/engine/objects"
why = "Turns the result into a flat list of objects"

[[needs]]
on = "/cluster/openapi"
why = "The schema it validates against"
+++
