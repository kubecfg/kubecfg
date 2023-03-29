# RFC02: Caching and vendoring

## Background

Jsonnet goes way out of its way to ensure that all the imports are well known statically, and that the evaluation is fully reproducible
and cacheable by providing side-effect free evaluation and avoiding computed imports.

Kubecfg adds the ability to directly import external dependencies. This is meant to be used for importing immutable external files,
usually links to some resource at some specific git sha. It also [supports](https://github.com/kubecfg/kubecfg/pull/218) explicitly enforcing a fingerprint check.

However, fetching external dependencies comes with problems:

* downloding external dependencies may be slow and annoying
* external files may disappear permanently or temporarily

## Proposal

We propose adding a download cache that will store a local copy of files downloaded from remote sources.

The cache directory structure, is suitable for checking-in the cache into the current VCS repo (if the user so chooses).

### Cache structure

Let's consider this jsonnet source file:

```jsonnet
import 'https://github.com/kubecfg/k8s-libsonnet/raw/144e62ccbdc9f7d4f8bccc14f33224cbf63e5185/k8s.libsonnet'
```

The cache directory can be set via the `--cache-dir` flag. By default its `<cachebase>/kubecfg`, where `<cachebase>` is determined by the [os.UserCacheDir](https://pkg.go.dev/os@go1.20.2#UserCacheDir).


After the first execution, kubecfg will create this directory structure

```console
$ tree ~/Library/Caches/kubecfg
/Users/mkm/Library/Caches/kubecfg
└── https
    └── github.com
        └── kubecfg
            └── k8s-libsonnet
                └── raw
                    └── 144e62ccbdc9f7d4f8bccc14f33224cbf63e5185
                        └── k8s.libsonnet
```

### Multiple cache sources

(optional, advanced)

The user can define multiple cache directories.
They will act as a "search path".
If a file is not found in the first directory, the next one will be tried until the cache directories are exhausted.
Only the first cache directory is used to write new files to.

Example:

```bash
kubecfg --cache-dir ./vendor --cache-dir ~/.cache/kubecfg show ....
```

Open questions:

* should we always put the default cache at the end of the cache search path?

### Vendoring

To enable vendoring the user needs to pass a custom location for the cache directory and choose a desirable cache pruning policy.

This plays well with the [Flags From Files](https://github.com/kubecfg/kubecfg/pull/224) feature.

```console
$ cat .kubecfgrc.jsonnet
{
    flags+: {
        'cache-dir': 'vendor',
        'cache-pruning-policy': 'vendor'
    },
}
$ kubecfg show foo.jsonnet
```

### Cache pruning

The simplest thing to do is to let the cache just grow and grow and let the user just clean it up when needed.

The other strategies are:

* size limit, delete least recently used
* remove all unreferenced
* ???

#### Remove all unreferenced strategy

One difficulty of the "remove all unreferenced" mechanism is that some repos contain multiple top level jsonnet files,
and if the cache pruning was executed after every invocation of the `kubecfg` command then the only cached/vendored files left would be 
the ones pertaining the last evaluated file.

I see a few options for this:

1. an explicit "prune" step where you tell kubecfg to look at all the jsonnet files and run the find-deps on them and delete all cache files that aren't visited.

2. at the beginning for the batch execution of multiple kubecfg commands, create a new (physical or logica) cache/vendor directory and filling this new cache by doing the same operation that you'd do if you were filling the cache after cache misses (except instead of downloading the files from remote sources we'd read them from the previous cache location that would be passed to the cache search path).

3. every kubecfg invocation of kubecfg will be passed a `--cache-access-log` flag which will reference a tmp file to which kubecfg will append the filenames of the cache files it has touched. At the end of the multi-file generation script, that file will be used to compute which cache files can be deleted (e.g. by callign a command like `kubecfg cache prune --not-in "${cache_access_log}"`)

### CI

When running in a CI, it's advisable to rely solely on the vendored files and break the build if the user
accidentally forgot to commit new imports:

```bash
kubecfg --ci ....
```

The `--ci` flag is intentionally generic because it may govern other behaviour that is sensible in a CI environment.

### Filename escaping

Naming files in a way that is safe despite having users that use different OSs like linux, macos and windows is quite harder than what it looks like.

Macos is case preserving but case insensitive (in all but the most weird setups some few people have). Some macos versions don't like the ":" char, some do.

Windows has a different path separator, supports far less punctuation, requires file names to be valid utf-8, and last but not least: has a set of odd reserved names that just cannot appear as filenames, things like `CON`, `NUL` are not valid file names!

I suggest we whitelist the printable **lowercase** characters that are usable on all three major OSs and uri-encode all of the others.
Yes, this means that uppercase letters will be uri-encoded, which is a bit ugly, but most file names developers write, in practice, don't have many upper case letters.
