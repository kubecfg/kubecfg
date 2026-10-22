+++
title = "CI"
summary = "Build, unit tests and integration tests on a kind cluster, for every push and PR"
aka = ["github actions", "go.yml", "kind"]
kind = "infra"
intent = "goal"
research = "done"
refs = [".github/workflows/go.yml", ".github/workflows/map.yml", "integration/"]

[[feature]]
id = "build-test"
title = "Build and unit tests"
status = "done"

[[feature]]
id = "integration"
title = "Integration tests on kind v0.33 with Kubernetes 1.37"
status = "done"

[[feature]]
id = "map-lint"
title = "todography lint --strict on this map"
status = "done"

[[feature]]
id = "permissions"
title = "Least-privilege GITHUB_TOKEN permissions"
status = "wip"
note = "PR #526; merging it needs a token with the workflow scope"

[[feature]]
id = "go-1-27"
title = "Build with Go 1.27"
status = "wip"
note = "PR #500; its TestPush failure was fixed in #534"
+++
