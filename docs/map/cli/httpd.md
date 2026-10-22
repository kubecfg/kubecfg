+++
title = "httpd"
summary = "Serve Jsonnet files as HTTP hooks: each POST evaluates a file with the request body"
aka = ["kubecfg httpd", "webhook"]
kind = "command"
intent = "goal"
research = "done"
status = "done"
refs = ["cmd/httpd.go", "pkg/kubecfg/httpd.go"]

[[needs]]
on = "/engine/vm"
why = "Evaluates the hook with the request as a top-level argument"

[[needs]]
on = "/config/feature-gates"
why = "Alpha: the httpd feature"
+++
