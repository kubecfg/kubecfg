+++
title = "Predefined kubecfg"
summary = "kubecfg is predefined in command-line code and .yakl files, not in Jsonnet files"
aka = ["RFC06", "predefined-kubecfg", "implicit-stdlib"]
kind = "lang"
intent = "aspiration"
research = "done"
refs = ["docs/rfcs/rfc06.md"]

[[feature]]
id = "import-fix"
title = "Imports work in eval -e, --ext-code and --tla-code"
status = "todo"

[[feature]]
id = "cli"
title = "kubecfg predefined in eval -e, --exec, --overlay-code, --ext-code, --tla-code"
status = "todo"
needs = ["#import-fix"]

[[feature]]
id = "yakl"
title = "kubecfg predefined in .yakl expressions, and printed by eval -o jsonnet"
status = "todo"
needs = ["/lang/yakl"]

[[feature]]
id = "hint"
title = "A hint after Unknown variable: kubecfg"
status = "todo"

[[needs]]
on = "/lang/stdlib"

[[needs]]
on = "/config/feature-gates"
why = "Starts as the alpha feature predefined-kubecfg"
+++

RFC06. Jsonnet files keep the explicit import, because editors, linters and
other Jsonnet tools only know `std`. Code that isn't in a file is only read by
kubecfg, so `kubecfg` can be predefined there.
