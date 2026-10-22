+++
title = "Jsonnet VM"
summary = "Sets up the Jsonnet VM: search paths, variables, the built-in library, overlays"
aka = ["vm", "jpath", "ext-str", "tla"]
kind = "engine"
intent = "induced"
research = "done"
refs = ["pkg/kubecfg/vm.go", "cmd/root.go"]

[[feature]]
id = "search-path"
title = "-J and -U library search paths, and KUBECFG_JPATH"
status = "done"

[[feature]]
id = "variables"
title = "External variables and top-level arguments, as strings, code or files"
status = "done"

[[feature]]
id = "input-formats"
title = "Jsonnet, YAKL, JSON and YAML inputs, chosen by file suffix"
status = "done"

[[feature]]
id = "builtin-lib"
title = "kubecfg.libsonnet built in, served from internal:///"
status = "done"
needs = ["/imports/universal#internal", "/lang/stdlib"]

[[feature]]
id = "overlays"
title = "--overlay-code and --overlay-code-file extend the input (the overlay feature)"
status = "done"
needs = ["/config/feature-gates"]

[[needs]]
on = "/deps/go-jsonnet"

[[needs]]
on = "/imports/universal"
why = "Every import goes through it"

[[needs]]
on = "/config/flags"
why = "Its settings come from the flags"
+++
