+++
title = "eval"
summary = "Evaluate a file or an expression and print any value, not only Kubernetes objects"
aka = ["kubecfg eval"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/eval.go", "pkg/kubecfg/eval.go", "docs/Basic-Usage/Kubecfg-Commands/eval.md"]

[[feature]]
id = "expr"
title = "-x: evaluate an expression against the file"
status = "done"

[[feature]]
id = "exec"
title = "-e: evaluate code without a file, as jsonnet -e and show -e do"
status = "done"

[[feature]]
id = "keys"
title = "-k: list an object's keys instead of rendering it"
status = "done"

[[feature]]
id = "jsonnet-out"
title = "-o jsonnet: print the Jsonnet a .yakl file compiles to"
status = "done"
needs = ["/lang/yakl"]

[[feature]]
id = "trace"
title = "--trace: show which source lines a value came from"
status = "done"
needs = ["/deps/ursonnet"]

[[feature]]
id = "compile-cmd"
title = "A separate compile command for YAKL"
status = "dropped"
note = "Too ad hoc for a top-level command; -o jsonnet replaced it"

[[needs]]
on = "/engine/vm"
why = "Evaluates the input"

[[needs]]
on = "/config/feature-gates"
why = "Alpha: the eval feature, first in line for beta"
+++
