+++
title = "Universal importer"
summary = "One importer for files, https, built-in libraries and the converting schemes"
aka = ["importer", "yaml:", "yakl:", "binary://", "internal://"]
kind = "engine"
intent = "induced"
research = "done"
refs = ["utils/importer.go"]

[[feature]]
id = "files"
title = "Relative paths and file:// against the search path"
status = "done"

[[feature]]
id = "http"
title = "http:// and https:// imports"
status = "done"

[[feature]]
id = "internal"
title = "internal:// serves the libraries built into kubecfg"
status = "done"

[[feature]]
id = "pipes"
title = "file:// imports from pipes such as /dev/stdin"
status = "done"

[[feature]]
id = "yaml-scheme"
title = "yaml: parses a YAML file"
status = "done"

[[feature]]
id = "yakl-scheme"
title = "yakl: and the .yakl suffix compile YAKL on the way in"
status = "done"
needs = ["/lang/yakl"]

[[feature]]
id = "docs-scheme"
title = "kubecfg+docs: wraps the value as {docs: v}"
status = "done"

[[feature]]
id = "binary"
title = "binary:// prefix (the binary-import feature), deprecated in favour of importbin"
status = "done"
note = "RFC05: candidate for removal"
needs = ["/config/feature-gates"]

[[needs]]
on = "/deps/go-jsonnet"
why = "Implements jsonnet.Importer"
+++
