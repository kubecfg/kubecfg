+++
title = "Apply order"
summary = "Best-effort dependency order, so objects arrive after the things they need"
aka = ["sort", "DependencyOrder"]
kind = "cluster"
intent = "induced"
research = "done"
status = "done"
refs = ["utils/sort.go"]

[[needs]]
on = "/cluster/openapi"
why = "Objects with a pod template go after the rest"
+++
