+++
title = "Pinned imports"
summary = "A #hash=sha256:... fragment on an import URL fails the import if the content changed"
aka = ["hashes", "#hash=", "pinned-imports"]
kind = "engine"
intent = "aspiration"
research = "done"
status = "done"
refs = ["utils/importer.go", "docs/Advanced-Usage/pinned-imports.md", "https://github.com/kubecfg/kubecfg/pull/218"]

[[needs]]
on = "/imports/universal"

[[needs]]
on = "/config/feature-gates"
why = "The pinned-imports feature, beta"
+++

The fragment holds hashes, as `hash=<algo>:<hex>`. Algorithms kubecfg doesn't
know are skipped, but at least one must be known, and every known one must
match. A fragment without `hash` isn't kubecfg's and gets a warning. New
behavior goes in import prefixes, like `yaml:`, not in the fragment.
