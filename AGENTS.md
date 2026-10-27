# Notes for coding agents

## Release notes

kubecfg has no CHANGELOG file. Release notes live in the GitHub release body. Use the previous release as the example (`gh release view <last tag> --json body -q .body`). List changes with `git log <last tag>..origin/main`, then read each PR.

Split the notes into two halves:

1. **Changes for everyone.** Only what a user sees without turning on a feature gate. Use these subsections and drop any that are empty: `### ✨ New`, `### 🐛 Fixes`, `### 📚 Docs and internals`. A `### ⚠️ Breaking changes` subsection is only for breaking changes here.
2. **Changes behind feature gates.** Start it with: "These changes only apply when the feature is turned on with `--features=<name>` or `--alpha`. Alpha features can change in any release." Then add one `### \`<feature>\` (<stage>)` section per gate that changed, with the stage from `pkg/features/features.go` (alpha, beta, removed, ...). Gates that didn't change get no section.

Changes to an alpha or beta feature never go under a breaking-changes heading, even when they break things (a rename, a removal). Put a short **Migrating:** paragraph inside that feature's section instead. That keeps the notes from looking scary to people who don't use the feature.

Link PRs as `(#123)`. Link docs to the release tag, e.g. `https://github.com/kubecfg/kubecfg/blob/v0.40.0/docs/...`.

## Cutting a release

Ask before tagging. Then:

1. `git tag -s -m vX.Y.Z vX.Y.Z origin/main` and `git push origin tag vX.Y.Z`.
2. The tag starts the `go.yml` workflow. It builds, pushes the images to ghcr, and creates a **draft** release with the binaries.
3. When the run passes: `gh release edit vX.Y.Z --title vX.Y.Z --notes-file <notes> --draft=false --latest`.

## RFCs

RFCs live in `docs/rfcs/`. A new one starts as **Draft**, and stays a draft when merged. Never mark one Accepted; see `docs/rfcs/README.md`.

## The map

`docs/map` is kubecfg's [todography](docs/map/README.md) map. A feature is `done` when it's on
main. A PR that fully implements a feature marks it `done` in the same commit, so the map is
right the moment the PR merges. A PR that only starts one marks it `wip`, with a note naming
the PR. Run `todography lint` after changing the map.
