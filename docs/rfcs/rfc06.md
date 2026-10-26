# RFC06: `kubecfg` predefined outside Jsonnet files

## Summary

To use kubecfg's library today, code has to import it first:

```jsonnet
local kubecfg = import 'kubecfg.libsonnet';
```

This RFC makes the name `kubecfg` available without that line, but only in Jsonnet code that isn't
in a Jsonnet file: code given on the command line, and expressions in `.yakl` files. Jsonnet files
(`.jsonnet`, `.libsonnet`) still need the import, so editors, linters and other Jsonnet tools can
still read them.

It's a new additive alpha feature, `predefined-kubecfg`. It replaces the "implicit `kubecfg` local"
follow-up in [RFC05](rfc05.md#an-implicit-kubecfg-local).

## Background

### Where the import costs something

In a Jsonnet file, the import is one line next to the file's other imports. Dropping it saves
almost nothing.

Other places have no good spot for it:

* **`eval -e`.** To list every object by kind, namespace and name:

  ```console
  $ kubecfg eval app.jsonnet -e 'local kubecfg = import "kubecfg.libsonnet"; kubecfg.fold(kubecfg.layouts.gvkNsName, $, {})'
  ```

  (This doesn't work today at all; see [Imports from command-line code](#imports-from-command-line-code).)
* **`--overlay-code`, `--exec`, `--ext-code`, `--tla-code`.** The same: the import has to be part
  of the one-line expression.
* **`.yakl` files.** The file needs a document just for the import, and its author has to know
  about file locals before calling one function:

  ```yaml
  '&kubecfg': ${import 'kubecfg.libsonnet'}
  ---
  apiVersion: v1
  kind: Pod
  metadata:
    name: web
  spec:
    containers:
    - name: web
      image: ${kubecfg.resolveImage('nginx:1.27')}
  ```

### Why not everywhere

RFC05 proposed making `kubecfg` predefined in every file. That has four problems.

1. **Editors and linters.** go-jsonnet has exactly one predefined name, `std`. It's written into
   its static check ([`static_analyzer.go`](https://github.com/google/go-jsonnet/blob/v0.22.0/internal/program/static_analyzer.go#L165)),
   and there's no API to add another. The C++ Jsonnet is the same. So every tool built on either
   one (language servers in editors, `jsonnet-lint`, `jsonnet`, Tanka) would report
   `Unknown variable: kubecfg` in a file that relied on the predefined name. go-jsonnet rejects the
   whole file at the first unknown name, before evaluating any of it, so a tool that evaluates the
   file gets nothing past that error. A missing `kubecfg.libsonnet` can be fixed in the tool's
   settings, by adding a search path or vendoring the file. A name the tool doesn't know can't.
2. **Libraries used by other tools.** The plain-Jsonnet parts of `kubecfg.libsonnet` (`fold`,
   `deepMap`, `toOverlay`, `layouts`, `getPath`) work in any Jsonnet tool if the file is vendored.
   `std.native` returns `null` for a function the tool doesn't have, so only the native functions
   fail, and only when called. If `kubecfg` were predefined in files, a library author could leave
   out the import and never notice, because the library would work in every kubecfg run. The first
   person to see the error would be someone using the library from another tool.
3. **`importstr`.** Without changing go-jsonnet, the only way to predefine a name in a file is to
   add text to the start of the file when it's imported. go-jsonnet asks the importer for a file in
   the same way for `import`, `importstr` and `importbin`, so the importer can't tell them apart.
   `importstr 'app.jsonnet'` would return the file with the added text, which changes the result
   of a file that works today. This is why YAKL needs the `yakl:` scheme. RFC05 suggested a
   go-jsonnet change that lets the program add names the way `std` is added. That fixes this
   problem, but not problems 1 and 2.
4. **The entry file only.** Predefining it only in the file named on the command line avoids
   problem 3. But then `base.jsonnet` works with `kubecfg show base.jsonnet` and fails when
   `prod.jsonnet` imports it. Whether a file works would depend on who loads it.

## The rule

> `kubecfg` is predefined in Jsonnet code that isn't in a Jsonnet file.

Editors and linters read files. Code that isn't in a file is only ever read by kubecfg, so no other
tool has to understand it. Code in a file is read by other tools, so it keeps the import.

| Where the code is | `kubecfg` predefined |
| --- | --- |
| `eval -e` | yes |
| `--exec` (`-e` on `show`, `update`, `diff`, `delete`, `validate`) | yes |
| `--overlay-code` | yes |
| `--ext-code`, `--tla-code` | yes |
| expressions in a `.yakl` file, however it's loaded (an argument, `import 'yakl:...'`, `--overlay-code-file`) | yes |
| a `.jsonnet` or `.libsonnet` file | no |
| a Jsonnet file named by `--overlay-code-file`, `--ext-code-file` or `--tla-code-file` | no |
| `httpd` hook files | no |

`--ext-str` and `--tla-str` values are strings, not code, so the rule doesn't apply to them.

## What "predefined" means

With the feature on, kubecfg evaluates the code as if it started with this line:

```jsonnet
local kubecfg = import 'kubecfg.libsonnet';
```

The rest follows from that:

1. **It never reaches into files.** In Jsonnet, an imported file doesn't see the locals of the code
   that imports it. So `kubecfg show -e 'import "base.jsonnet"'` doesn't make `kubecfg` available
   inside `base.jsonnet`.
2. **It's the same file an explicit import finds.** The import is resolved from where the code is,
   exactly as if it were written there. For `eval -e`, that's the directory of the evaluated file.
   For other command-line code, it's the current directory. For a `.yakl` file, it's the file's
   directory. After that come the search path (`-J`, `-U`, `KUBECFG_JPATH`) and the copy built
   into kubecfg. A vendored `kubecfg.libsonnet` on the search path wins, as it does for an explicit
   import. So `kubecfg` and `import 'kubecfg.libsonnet'` are never two different files in the same
   run.
3. **Your own names win.** A `local`, a function parameter or an object local named `kubecfg` hides
   it, as with any Jsonnet local. In a `.yakl` file, an anchor (`&kubecfg`) or a local key
   (`'&kubecfg':`) hides it. In `eval -e`, so does a top-level argument named `kubecfg`.
4. **It's only loaded if used.** Jsonnet evaluates a local only when something reads it. Code that
   doesn't use `kubecfg` doesn't load the library and behaves exactly as before.
5. **Line numbers in errors don't change.** The line goes in front of the code, on its first line,
   so only the columns of that first line move. (For `eval -e` the columns already don't match what
   you typed, because kubecfg wraps the expression in code of its own. Compiled YAKL doesn't keep
   columns exactly either.)

### In `.yakl` files

The YAKL compiler adds `kubecfg` as a file local. It's the same local that this local key declares
in a document of local keys:

```yaml
'&kubecfg': ${import 'kubecfg.libsonnet'}
```

The compiler adds the local itself, not a document. A document in front of the file would break a
file whose first document is `<<+: S`, because that document has to be the first.

It does this only when both of these are true:

* the word `kubecfg` appears in one of the file's expressions, outside strings and comments;
* the file doesn't name anything `kubecfg` itself, with an anchor or a local key.

A file that doesn't use `kubecfg` compiles to the same Jsonnet as before. A field access such as
`x.kubecfg` also counts as a use. That only adds a local that nothing reads.

File locals never change the shape of a YAKL file's value (a document, a list or an object), so the
added local doesn't either. Line numbers stay as they are: the compiler writes the local in front of
line 1 and doesn't add a line.

`kubecfg eval -o jsonnet` prints the Jsonnet with the import in it. For the Pod above, without its
first document, it prints the same thing it prints today for the file with that document:

```jsonnet
local kubecfg = import 'kubecfg.libsonnet';
{
  apiVersion: 'v1',
  kind: 'Pod',
  metadata: {
    name: 'web',
  },
  spec: {
    containers: [
      {
        name: 'web',
        image: kubecfg.resolveImage('nginx:1.27'),
      },
    ],
  },
}
```

So if you save the output as a `.jsonnet` file, the file has its import and follows the rule.

## Feature gate

`predefined-kubecfg` is a new feature at **alpha**, of the **additive** kind:

```go
PredefinedKubecfg = register(Feature{Name: "predefined-kubecfg", Stage: Alpha, Since: "v0.42.0",
	Doc: "kubecfg is predefined in command-line code and .yakl files"})
```

* **Why it's additive.** Today a free `kubecfg` fails with `Unknown variable: kubecfg`, and the
  check runs before evaluation. So no code that works today mentions it. Turning the feature on
  only changes code that fails today. Under RFC05 it can go from alpha to beta to GA.
* **Why it has its own gate.** `--exec`, `--ext-code` and `--tla-code` are stable flags. Nothing
  that works with them changes, but once the name is available there without a gate, it can't be
  taken away again. The gate keeps the design open while it's alpha. It also gives the feature one
  stage everywhere, instead of one per command.
* **With other gates.** `eval -e`, `--overlay-code` and YAKL are alpha features themselves, so
  there both gates must be on, for example `--features=yakl,predefined-kubecfg`. `--alpha` turns
  on both.
* **With the feature off,** nothing changes.
* **OCI bundles.** `kubecfg pack` stores `.yakl` files as source, and the program that imports the
  bundle compiles them. A bundle with a `.yakl` file that uses the predefined `kubecfg` needs the
  feature on there too, as it already needs `yakl`. RFC05's follow-up on recording features in
  bundles covers this.

## Imports from command-line code

This has to be fixed first. Today, imports don't work in `eval -e`, `--ext-code` and `--tla-code`,
not even explicit ones:

```console
$ kubecfg --features=eval eval app.jsonnet -e "(import 'kubecfg.libsonnet').isK8sObject({})"
RUNTIME ERROR: Get "/kubecfg.libsonnet": unsupported protocol scheme ""
```

The cause: kubecfg's importer resolves a relative import against the name of the code doing the
import. For a file, that name is a `file://` URL. For this code it isn't a URL. `eval -e` is named
after the path of the evaluated file, and go-jsonnet names `--ext-code` and `--tla-code` values
`<extvar:NAME>` and `<top-level-arg:NAME>`. Resolving against such a name gives a URL with no
scheme. The importer tries to fetch it, fails, and reports that as an error instead of trying the
search path. `--exec` and `--overlay-code` are named with `data:` URLs, so they work.

The fix: when the importing code's name has no scheme, treat it as a path relative to the current
directory. Then `eval -e` imports relative to the evaluated file, and `--ext-code` and `--tla-code`
import relative to the current directory, as the `jsonnet` command does. The search path comes
after that, as usual.

This is a bug fix, not part of the feature. It isn't behind the gate, and it can land first.

## Error message

Because of the rule, code that works in `-e` can fail when it's moved into a file. To make that
quick to fix, when evaluation fails with `Unknown variable: kubecfg`, kubecfg adds a hint after the
error.

With the feature on, the error must come from a Jsonnet file:

```
hint: kubecfg is not predefined in Jsonnet files. Add this line at the top of the file:
  local kubecfg = import 'kubecfg.libsonnet';
```

With the feature off, the hint has one more line:

```
  In command-line code and .yakl files, --features=predefined-kubecfg predefines it.
```

## Alternatives

* **Everywhere.** See [Why not everywhere](#why-not-everywhere).
* **Only in kubecfg's own places** (`eval -e`, `--overlay-code`, YAKL), leaving out `--exec`,
  `--ext-code` and `--tla-code` because the `jsonnet` command has the same flags. It would need no
  new gate, since those three places are behind alpha gates already. But `kubecfg eval -e` and
  `kubecfg show -e` would disagree, and the rule would become a list to remember instead of one
  sentence.
* **Bind the built-in copy directly** (`import 'internal:///kubecfg.libsonnet'`). This skips the
  lookup, and so also skips the import bug. But a vendored copy on the search path would no longer
  win, so `kubecfg` and an explicit import could be two different files in the same run.
* **A go-jsonnet change to add predefined names** (RFC05's plan). It fixes `importstr`, but not
  editors, linters or shared libraries. This RFC doesn't need it: every place it covers is code
  that kubecfg already wraps or compiles.

## Implementation sketch

* `utils/importer.go`, `expandImportToCandidateURLs`: an importing name without a scheme is a path
  relative to the current directory. This is the fix, and it lands first.
* `pkg/features/features.go`: register `predefined-kubecfg`.
* One helper that puts `local kubecfg = import 'kubecfg.libsonnet'; ` in front of a piece of code
  when the feature is on. It's called wherever kubecfg builds code from a flag:
  * `pkg/kubecfg/eval.go`: in front of the whole `evalSnippet`, so a top-level argument named
    `kubecfg` still wins; and in front of the expression that `--trace` gives to ursonnet.
  * `cmd/root.go`, `readObjs`: `--exec`.
  * `pkg/kubecfg/vm.go`, `ReadObjects`: in front of the whole overlay expression. This is safe
    because the input file and an `--overlay-code-file` are imported, and imports don't see it
    (point 1 above).
  * `pkg/kubecfg/vm.go`, `JsonnetVM`: `--ext-code` and `--tla-code` values. Not `--ext-str` or
    `--tla-str`, which are strings. For the `*-file` flags the value is an import, which the line
    doesn't reach.
* `pkg/yakl`: `Compile` and `Desugar` take an option to predefine `kubecfg`. The importer passes it
  when the feature is on, and so does `eval -o jsonnet`. The compiler already finds the words in
  each expression (`words`), which is all the "is it used" check needs.
* The hint, where kubecfg prints errors.
* Tests:
  * each row of the table, with the feature on and off;
  * a `.yakl` file that uses `kubecfg`, one that doesn't (its output doesn't change), and one that
    declares its own;
  * a `.jsonnet` file that uses `kubecfg` still fails, with the hint;
  * imports from `eval -e`, `--ext-code` and `--tla-code`, with the feature off.
* Docs: `eval.md`, `yakl.md`, the feature gates page, and the map entry `t8y:/lang/implicit-stdlib`.

## Open questions

* **The name.** `predefined-kubecfg` says what you get. RFC05 used `implicit-stdlib`, which could be
  read as Jsonnet's `std`.
* **`--ext-code` and `--tla-code`.** These are the `jsonnet` command's flags. Including them keeps
  the rule one sentence (see [Alternatives](#alternatives)). They could also be left out now and
  added later, which would be additive too.
