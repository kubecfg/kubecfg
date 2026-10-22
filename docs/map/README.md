# The kubecfg map

One Markdown file per component, with TOML front matter, in the
[todography](https://github.com/mkmik/todography) format. It records what
kubecfg has, what is being built, and what we'd like to have.

```console
todography lint        # is the map consistent?
todography next --all  # what's ready, and what's blocked by what
todography open        # the interactive page
```

In comments, commit messages and docs, refer to a component as
`t8y:/area/component`.
