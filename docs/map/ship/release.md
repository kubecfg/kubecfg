+++
title = "Release"
summary = "Push a version tag; CI makes the GitHub release, binaries and images"
aka = ["tag", "ko", "ghcr"]
kind = "infra"
intent = "goal"
research = "done"
refs = [".github/workflows/go.yml"]

[[feature]]
id = "binaries"
title = "A GitHub release with a binary per platform"
status = "done"

[[feature]]
id = "images"
title = "Multi-arch container images built with ko, pushed to ghcr"
status = "done"

[[needs]]
on = "/ship/ci"
why = "The release jobs are part of the CI workflow"
+++

The latest release is v0.39.0. Homebrew packages it separately.
