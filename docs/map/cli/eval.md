+++
title = "eval"
summary = "Read the inputs as show does and print their values, not only Kubernetes objects"
aka = ["kubecfg eval"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/eval.go", "pkg/kubecfg/eval.go", "docs/Basic-Usage/Kubecfg-Commands/eval.md"]

[[feature]]
id = "inputs"
title = "The same inputs as show: files, -e, overlays, -x and top-level arguments, from the same code"
status = "done"

[[feature]]
id = "expr"
title = "-x: evaluate an expression against each input"
status = "done"

[[feature]]
id = "exec"
title = "-e: code as one more input, as jsonnet -e and show -e do"
status = "done"

[[feature]]
id = "keys"
title = "-k: list an object's keys instead of rendering it"
status = "done"

[[feature]]
id = "jsonnet-out"
title = "-o jsonnet: print the Jsonnet a .yakl file compiles to; part of the yakl feature"
status = "done"
needs = ["/lang/yakl"]

[[feature]]
id = "trace"
title = "--trace: show which source lines a value came from; part of the traceback feature"
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
