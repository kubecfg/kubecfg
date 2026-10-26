# Pinned imports

An import URL can carry the hash of the content it should return. If the content has a different
hash, the import fails:

```jsonnet
local lib = import 'https://raw.githubusercontent.com/kubecfg/kubecfg/v0.29.1/testdata/configmap.jsonnet#hash=sha256:7871c163d6071da0289f4f20276f9e2a728ad2f82bd7e68361f986b53a585f22';
```

kubecfg can't tell whether a URL will always return the same bytes. A URL with a commit hash in it
is meant to be fixed, but kubecfg would have to know each hosting site's URL format to check that,
and a tag like `v0.29.1` can be moved. A hash in the URL works the same for every server: kubecfg
checks the bytes it got against it.

It works for every kind of import: `https://`, `file://`, `oci://`, relative paths, `yaml:` and
`yakl:` imports, `importstr` and `importbin`, and the file you pass on the command line:

```console
$ kubecfg show 'file:///path/to/app.jsonnet#hash=sha256:...'
```

For `yaml:` and `yakl:` imports the hash is of the file itself, not of what kubecfg makes from it.

To get the hash of a file, run `sha256sum` (or `shasum -a 256`) on it. Or pin it with an empty hash
and copy the right one from the error:

```console
$ kubecfg show 'https://example.com/app.jsonnet#hash=sha256:'
error reading https://example.com/app.jsonnet#hash=sha256:: https://example.com/app.jsonnet#hash=sha256:: content hash mismatch: got sha256:<the hash>, want sha256:
```

This is the `pinned-imports` [feature](feature-gates.md). It's in beta, so it's on unless you turn
it off with `--features=pinned-imports=false`. It will stay in beta for a long time. If you have an
import whose fragment reads `hash=sha256:<hex>` but isn't meant as the hash of its content, please
[open an issue](https://github.com/kubecfg/kubecfg/issues): we'd like to know. kubecfg v0.41.1 and older don't know about it: they
import the content without checking the hash.

## The fragment

The part of a URL after `#` is the fragment. Clients never send it to the server, so it's up to
kubecfg to decide what it means. kubecfg reads it like a query string (`key=value&key=value`).

- A fragment without a `hash` key isn't meant for kubecfg. kubecfg prints a warning and otherwise
  ignores it, as it always did.
- `hash` has the form `<algorithm>:<hex>`. kubecfg knows `sha256` and `sha512`.
- `hash` can be repeated, once per algorithm: `#hash=sha256:...&hash=sha512:...`. Every hash
  kubecfg knows must match.
- kubecfg skips algorithms it doesn't know. A later kubecfg may add one, and a URL that lists it
  next to `sha256` still works with this version.
- At least one algorithm kubecfg knows must be there, or the import fails. That way a typo like
  `sha265`, or a hash that only a newer kubecfg can check, can't quietly turn the check off.
- In a fragment with `hash`, any other key is an error.
