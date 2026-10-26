# RFC06: `kubecfg` predefined outside Jsonnet files

## Summary

To use kubecfg's library, code has to import it first:

```jsonnet
local kubecfg = import 'kubecfg.libsonnet';
```

This RFC makes the name `kubecfg` available without that line, but only in Jsonnet code that isn't
in a Jsonnet file: code given on the command line, and expressions in `.yakl` files. Jsonnet files
(`.jsonnet`, `.libsonnet`) still need the import, so editors, linters and other Jsonnet tools can
still read them.

The predefined `kubecfg` is always the copy of the library built into kubecfg. It isn't behind a
feature gate: code that works today is evaluated exactly as before, and only code that fails with
`Unknown variable: kubecfg` changes. This RFC replaces the "implicit `kubecfg` local" follow-up in
[RFC05](rfc05.md#an-implicit-kubecfg-local).

## Background

### Where the import costs something

In a Jsonnet file, the import is one line next to the file's other imports. Dropping it saves
almost nothing.

Other places have no good spot for it:

* **`eval -e`.** To list every object by kind, namespace and name:

  ```console
  $ kubecfg eval app.jsonnet -e 'local kubecfg = import "kubecfg.libsonnet"; kubecfg.fold(kubecfg.layouts.gvkNsName, $, {})'
  ```

  (Before this RFC, this failed even with the import; see
  [Imports from command-line code](#imports-from-command-line-code).)
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

kubecfg checks the code first. If the code reads the variable `kubecfg` without declaring it,
kubecfg evaluates it as if it started with this line:

```jsonnet
local kubecfg = import 'internal:///kubecfg.libsonnet';
```

`internal:///kubecfg.libsonnet` is the copy of `kubecfg.libsonnet` built into kubecfg. The rest
follows from that line:

1. **It's always the built-in copy.** Neither the search path (`-J`, `-U`, `KUBECFG_JPATH`) nor a
   file next to the code changes it, in the same way that they can't change `std`. Why:
   * Nothing in the code says which file the name comes from, so no file should be able to change
     it. An explicit import is different: you wrote it, and it follows Jsonnet's lookup rules as
     before.
   * Much of the library wraps native functions compiled into kubecfg. The built-in copy always
     matches them. A vendored copy can be older or newer than the kubecfg running it.
   * People vendor `kubecfg.libsonnet` so that other tools can find it when a Jsonnet file imports
     it. Those files keep their import, and it still finds the vendored copy. The predefined name
     only exists in code that other tools never read.
   * There's nothing to look up. A lookup can fetch a `-U` URL, or a file next to a `.yakl` file
     that was loaded over `https://` or from an OCI bundle, and it can fail. The built-in copy is
     the same everywhere.

   So one run can use two copies: the built-in one through the predefined name, and a vendored one
   through an explicit import. They don't interfere: Jsonnet code has no side effects and no shared
   state, so a value from one copy works with functions from the other. They can only differ in
   what their functions return, if their versions differ. To use another copy, import it yourself:
   your import hides the predefined name (point 3).
2. **It never reaches into files.** In Jsonnet, an imported file doesn't see the locals of the code
   that imports it. So `kubecfg show -e 'import "base.jsonnet"'` doesn't make `kubecfg` available
   inside `base.jsonnet`.
3. **Your own names win.** A `local`, a function parameter or an object local named `kubecfg` hides
   it, as with any Jsonnet local. In a `.yakl` file, an anchor (`&kubecfg`) or a local key
   (`'&kubecfg':`) hides it. In `eval -e`, so does a top-level argument named `kubecfg`.
4. **Code that works today doesn't change.** The line is only added to code that reads `kubecfg`
   without declaring it, and today that code fails with `Unknown variable: kubecfg`. Everything
   else is evaluated exactly as before, down to the columns in its error messages. kubecfg finds
   out with Jsonnet's own check, the one that reports `Unknown variable`, so the answer is exact: a
   field named `kubecfg`, as in `x.kubecfg`, or the word in a string or a comment, isn't a use.
5. **Line numbers in errors don't change.** The line goes in front of the code, on its first line,
   so only the columns of that first line move. (For `eval -e` the columns already don't match what
   you typed, because kubecfg wraps the expression in code of its own. Compiled YAKL doesn't keep
   columns exactly either.)

### In `.yakl` files

The YAKL compiler checks the compiled file in the same way. If the file reads `kubecfg` without
declaring it, the compiler adds the local. It means the same as this local key in a document of
local keys:

```yaml
'&kubecfg': ${import 'internal:///kubecfg.libsonnet'}
```

The compiler adds the local itself, not a document. A document in front of the file would break a
file whose first document is `<<+: S`, because that document has to be the first.

A file that doesn't use `kubecfg`, or that declares it wherever it uses it, for example with
`'&kubecfg':`, compiles to the same Jsonnet as before.

A local never changes the shape of a YAKL file's value (a document, a list or an object), so the
added one doesn't either. Line numbers stay as they are: the compiler writes the local in front of
the code of line 1 and doesn't add a line.

`kubecfg eval -o jsonnet` prints the Jsonnet with the import in it, after the file's first comments.
For the Pod above, without its first document, it prints:

```jsonnet
local kubecfg = import 'internal:///kubecfg.libsonnet';
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

It's the code that kubecfg evaluates, so it names the built-in copy. Saved as a `.jsonnet` file, it
works in kubecfg as it is. For other Jsonnet tools, change the import to `'kubecfg.libsonnet'` and
vendor the library, as for any Jsonnet file that uses it.

## No feature gate

The rule is on for everyone, without a feature gate:

* **It's additive in the strictest sense.** By point 4, code that works today is evaluated exactly
  as before. Only code that fails today with `Unknown variable: kubecfg` changes, and it starts
  working. RFC05 calls such features additive.
* **A gate would cancel the gain.** The point is to type less on the command line. Typing
  `--features=predefined-kubecfg` instead of the import saves almost nothing, and in YAKL files it
  would mean two gates.
* **The unstable parts are gated anyway.** `eval`, `--overlay-code` and YAKL are alpha features, so
  the rule can still change there while they are alpha. What becomes stable right away is the rule
  in `--exec`, `--ext-code` and `--tla-code`, and it can't be taken back from those flags later.
  That's accepted: the reason for the rule, that no other tool reads command-line code, holds for
  them as it does for `eval -e`.
* **OCI bundles.** `kubecfg pack` stores `.yakl` files as source, and the kubecfg that imports the
  bundle compiles them. A bundle with a `.yakl` file that uses the predefined `kubecfg` needs a
  kubecfg with this RFC, as it already needs one with `yakl` turned on.

## Imports from command-line code

Before this RFC, imports didn't work in `eval -e`, `--ext-code` and `--tla-code`, not even
explicit ones:

```console
$ kubecfg --features=eval eval app.jsonnet -e "(import 'kubecfg.libsonnet').isK8sObject({})"
RUNTIME ERROR: Get "/kubecfg.libsonnet": unsupported protocol scheme ""
```

The cause: go-jsonnet tells kubecfg's importer the directory of the code doing the import, taken
from the code's name, and the importer resolves a relative import against it. For a file, that
directory is a `file://` URL. For this code it isn't a URL. `eval -e` is named after the path of
the evaluated file, so its directory is a path like `dir/`, or `""` for a file in the current
directory. go-jsonnet names `--ext-code` and `--tla-code` values `<extvar:NAME>` and
`<top-level-arg:NAME>`, so their directory is `""`. Resolving against a path gives a URL with no
scheme. The importer tried to fetch it, failed, and reported that as an error instead of trying the
search path. `--exec` and `--overlay-code` are evaluated from the current directory as a
`file://` URL, so they worked.

The fix: a directory without a scheme is a path relative to the current directory. So `eval -e`
imports relative to the evaluated file, and `--ext-code` and `--tla-code` import relative to the
current directory, as the `jsonnet` command does. The search path comes after that, as usual.

The predefined name doesn't need this fix, because its import is an absolute URL. Explicit imports
in the same code do, and an explicit import is how you use a copy other than the built-in one.

## Error message

Because of the rule, code that works in `-e` can fail when it's moved into a file. To make that
quick to fix, when kubecfg fails with `Unknown variable: kubecfg`, it prints a hint after the error.
Everywhere else `kubecfg` is predefined, so the code that failed is in a Jsonnet file:

```
hint: kubecfg is not predefined in Jsonnet files. Add this line at the top of the file:
  local kubecfg = import 'kubecfg.libsonnet';
```

The hint suggests `'kubecfg.libsonnet'`, not `internal:///`, because a Jsonnet file should keep an
import that other tools can find.

## Alternatives

* **Everywhere.** See [Why not everywhere](#why-not-everywhere).
* **Only in kubecfg's own places** (`eval -e`, `--overlay-code`, YAKL), leaving out `--exec`,
  `--ext-code` and `--tla-code` because the `jsonnet` command has the same flags. Nothing stable
  would change. But `kubecfg eval -e` and `kubecfg show -e` would disagree, and the rule would
  become a list to remember instead of one sentence.
* **Resolve it like an explicit import** (`import 'kubecfg.libsonnet'`, through the search path).
  Then `kubecfg` would be the same file as an explicit import at the same spot. But a file on the
  search path or next to the code would change what a name you never imported means, and the lookup
  can fetch URLs and fail. See point 1.
* **Behind a feature gate.** See [No feature gate](#no-feature-gate).
* **A go-jsonnet change to add predefined names** (RFC05's plan). It fixes `importstr`, but not
  editors, linters or shared libraries. This RFC doesn't need it: every place it covers is code
  that kubecfg already wraps or compiles.

## Implementation

* `utils/importer.go`, `expandImportToCandidateURLs`: a directory without a scheme is a path
  relative to the current directory. This is the import fix.
* `pkg/yakl`: `KubecfgLocal` is the line. `PredefineKubecfg(code)` puts it in front of code that
  reads `kubecfg` without declaring it, and returns any other code as it is. The check is
  `freeVars`, which the compiler already used to find the variables that a key reads. `Compile`
  and `Desugar` do the same for a yakl file, after its first comments.
* `yakl.PredefineKubecfg` is called wherever kubecfg builds code from a flag:
  * `pkg/kubecfg/eval.go`: on the whole `eval -e` snippet, so a top-level argument named `kubecfg`
    still wins; and on the expression that `--trace` gives to ursonnet.
  * `cmd/root.go`, `readObjs`: `--exec`.
  * `pkg/kubecfg/vm.go`, `ReadObjects`: the whole `--overlay-code` expression. This is safe because
    the input file is imported, and imports don't see the line (point 2).
  * `pkg/kubecfg/vm.go`, `JsonnetVM`: `--ext-code` and `--tla-code` values. Not `--ext-str` or
    `--tla-str`, which are strings. For the `*-file` flags the value is an import, which the line
    doesn't reach.
* `cmd.Hint` makes the hint, and `main.go` prints it after the error.
* Tests: each row of the table, names that hide the predefined one, a vendored copy on `-J` that
  the predefined name ignores and an explicit import finds, `.yakl` files that use `kubecfg`,
  don't, or declare their own, a `.jsonnet` file that still fails, with the hint, and imports from
  `eval -e`, `--ext-code` and `--tla-code`.
* Docs: `eval.md`, `show.md`, `yakl.md`, and the map entry `t8y:/lang/implicit-stdlib`.

## Follow-ups

* **A flag that replaces the built-in library,** for example to try a changed `kubecfg.libsonnet`
  without building kubecfg. It would replace what `internal:///kubecfg.libsonnet` serves, so it
  would change the built-in library everywhere: the predefined name, explicit `internal:///`
  imports, and the last place that `import 'kubecfg.libsonnet'` looks. It wouldn't depend on `-J`.
  It adds something without changing anything that works, so it can come later.
