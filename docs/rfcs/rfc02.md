# RFC02: Flags From Files (FFF)

## Background

(when we're speaking about flags here, we're also talking about flags mapped from env variables)

Kubecfg's behaviour is affected by the presence of some flags.
Flags can be either passed on the CLI or taken from env variables mapped to the flags.
Flags can be problematic; not all flags do, let me tell you why.

Some flags are part of the "interactive experience".
They are effectively part of the language through which users asks kubecfg to do something for them.

Some flags are user preferences. 

But other flags just must be there or Jsonnet just doesn't evaluate correctly.
Those flags are problematic.
When the user needs to use them, they stray from the "kubecfg way".

They are problematic because if they must be there, the user has two options:

1. Remember to provide them every time they invoke the command
2. Create a kubecfg wrapper script (or an env file) and remember to always use it.

I've seen a lot of people doing (2), with a wrapper script that sets up all the import-path flags
to their vendored dependencies, passing ext vars etc etc.

Once the user does that, it effectively puts vital config information into a language that not particularly well suited
for describing configs, namely, shell scripts or makefiles or whatnot.

## Proposal

Right before starting up, search for a `.kubecfgrc.jsonnet` in the current dir, and recursively in the parent directories.

If the file is found, evaluate it and use it to populate the default values of any flags.

The `kubecfgrc.jsonnet` will be overlaid against this file:

```jsonnet
{
    flags: { },
    commands: {
        update: $.flags,
        validate: $.flags,
        ....
    } 
}
```

Every time a command reads the value of a flag it, it will look it up with `commands[commandName][flagName]`,
and use that value unless the user overrides via a CLI flag.

Both CLI and env win over `.kubecfgrc.jsonnet`.

In other words the kubecfgrc flags are the default values for the flags.

Example:

```console
$ cat .kubecfgrc.jsonnet
{
    commands+: {
        show+: {
            format: 'json',
            jpath: ['testdata'],
        }
    }
}
$ kubecfg show -e "import 'configmap.jsonnet'"
{
  "apiVersion": "v1",
  "kind": "ConfigMap",
  "metadata": {
    "name": "test"
  }
}
```

### Paths

Some flags such as `--jpath` contain path names. When a relative path is currently provided to such flags, it is interpreted relatively
to the current directory at the time of executing the `kubecfg` command. We can find those flags because they are marked with an annotation, see `cobra.MarkFlagFilename`.

However, in the context of a `.kubecfgrc.jsonnet` file, it would be very practical to resolve such path names relative to the location of the `.kubecfgrc.jsonnet` file.

This is necessary to meet our goals: we'd like the end user to just `kubecfg show some/file.jsonnet` and have that just work.
Forcing the user to always run the kubecfg command for a specific directory defeats the purpose.

This is going to be more and more important as we add some more features like caching+vendoring.

### Multiple files

Most kubecfg commands can operate on multiple files.
When presented with multiple files, commands like show, update, delete, diff will evaluate each of them independently.
Ideally, the location of the `.kubecfgrc.jsonnet` should be relative to the jsonnet file itself, not the current dir where the user happens to run `kubecfg` from.
However, while each of the evaluation is independent, some flags may only make sense for the whole command invocation.


I don't really know how often people invoke kubecfg on multiple file given that The Kubecfg Way is about having a single root file that "wires up" an entire deployment.
I don't know which flags would be problematic if we'd use different flag values for each file evaluation during a single command execution.

But I know it would be hard to reason about and hard to implement.
Hence I propose a simple rule: raise an error if while evaluating multiple files, different `.kubecfgrc.jsonnet` files result from searches starting from the input jsonnet files. I.e.

```console
$ testdata/fffbad
├── .kubecfgrc.jsonnet
├── one
│   └── demo.jsonnet
└── two
    ├── .kubecfgrc.jsonnet
    └── demo.jsonnet
$ kubecfg show testdata/fffbad/{one,two}/demo.jsonnet
ERROR all evaluated files should resolve to the same .kubecfgrc.jsonnet, found ["testdata/fffbad/two/.kubecfgrc.jsonnet" "testdata/fffbad/.kubecfgrc.jsonnet"] instead
```
