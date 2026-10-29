+++
title = "show"
summary = "Render the input files as a YAML or JSON stream of Kubernetes objects"
aka = ["kubecfg show", "export-dir"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/show.go", "pkg/kubecfg/show.go", "docs/Basic-Usage/Kubecfg-Commands/show.md"]

[[feature]]
id = "render"
title = "YAML or JSON output (-o)"
status = "done"

[[feature]]
id = "expr"
title = "-x: show the objects in an expression against each input; part of the eval feature"
status = "done"

[[feature]]
id = "export-dir"
title = "--export-dir: one file per object, named by a Go template"
status = "done"

[[feature]]
id = "provenance"
title = "--show-provenance: annotate each object with the file and field it came from"
status = "done"
needs = ["/engine/objects#provenance"]

[[feature]]
id = "reorder-server"
title = "--reorder=server: put objects in the order update applies them"
status = "done"
needs = ["/cluster/ordering"]

[[feature]]
id = "reorder-client"
title = "--reorder=client: the same order without asking a cluster"
status = "todo"
note = "The flag's help text still says TODO"

[[needs]]
on = "/engine/vm"
why = "Evaluates the input files"

[[needs]]
on = "/engine/objects"
why = "Turns the result into a flat list of objects"
+++
