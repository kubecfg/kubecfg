+++
title = "yaklfmt"
summary = "Format the Jsonnet expressions inside .yakl files"
aka = ["kubecfg yaklfmt"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/yaklfmt.go", "pkg/yakl/format.go"]

[[feature]]
id = "format"
title = "Print the file with its expressions formatted"
status = "done"

[[feature]]
id = "in-place"
title = "-i: rewrite the files"
status = "done"

[[needs]]
on = "/lang/yakl"
why = "Finds the expressions"

[[needs]]
on = "/deps/go-jsonnet"
why = "Its formatter formats each expression"

[[needs]]
on = "/config/feature-gates"
why = "Alpha: the yakl feature"
+++
