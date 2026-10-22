+++
title = "Metacontroller hooks"
summary = "Write metacontroller sync hooks in Jsonnet and serve them with kubecfg httpd"
aka = ["metacontroller"]
kind = "embed"
intent = "goal"
research = "done"
status = "done"
refs = ["docs/Advanced-Usage/metacontroller-integration.md", "examples/metacontroller"]

[[needs]]
on = "/cli/httpd"
why = "Serves the hooks"
+++
