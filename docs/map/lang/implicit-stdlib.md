+++
title = "Predefined kubecfg"
summary = "kubecfg is predefined in command-line code and .yakl files, not in Jsonnet files"
aka = ["RFC06", "implicit-stdlib"]
kind = "lang"
intent = "goal"
research = "done"
refs = ["https://github.com/kubecfg/kubecfg/pull/554", "docs/rfcs/rfc06.md", "pkg/yakl/yakl.go", "utils/importer.go", "cmd/root.go"]

[[feature]]
id = "import-fix"
title = "Imports work in eval -x, --ext-code and --tla-code"
status = "done"

[[feature]]
id = "cli"
title = "kubecfg predefined in eval -x, --exec, --overlay-code, --ext-code, --tla-code"
status = "done"

[[feature]]
id = "yakl"
title = "kubecfg predefined in .yakl expressions, and printed by eval -o jsonnet"
status = "done"
needs = ["/lang/yakl"]

[[feature]]
id = "builtin"
title = "Always the built-in copy (internal:///), whatever the search path holds"
status = "done"

[[feature]]
id = "hint"
title = "A hint after Unknown variable: kubecfg"
status = "done"

[[feature]]
id = "override"
title = "A flag that replaces the built-in kubecfg.libsonnet"
status = "todo"

[[needs]]
on = "/lang/stdlib"
+++

RFC06. Jsonnet files keep the explicit import, because editors, linters and
other Jsonnet tools only know `std`. Code that isn't in a file is only read by
kubecfg, so `kubecfg` can be predefined there. It's added only where the code
would otherwise fail, so it needs no feature gate.
