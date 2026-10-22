+++
title = "Renovate"
summary = "Renovate opens PRs to keep the Go modules and actions current"
aka = ["renovatebot", "dependency updates"]
kind = "infra"
intent = "goal"
research = "done"
refs = [".github/renovate.json", ".github/workflows/renovate.yml"]

[[feature]]
id = "running"
title = "Dependency and security update PRs"
status = "done"

[[feature]]
id = "grouping"
title = "Group the Kubernetes modules; merge patch and digest updates without review"
status = "wip"
note = "PR #527"
+++
