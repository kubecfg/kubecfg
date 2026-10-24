# Experimental examples

Examples of kubecfg's alpha features, one directory per feature. Each directory has a
`.kubecfgrc.jsonnet` that turns on the features its examples need, so they run without
flags:

```
kubecfg show yakl/guestbook.yakl
kubecfg show knit/shop/prod.yakl
```

| Directory | Feature | What it shows |
| --- | --- | --- |
| `yakl/` | [YAKL](../../docs/Advanced-Usage/yakl.md) | the guestbook frontend in YAML with Jsonnet expressions |
| `knit/` | [knit](../../docs/Advanced-Usage/knit.md) | the guestbook on knit |
| `knit/shop/` | knit and YAKL | a shop in plain Kubernetes YAML, with dev and prod overlays |

`kubecfg features` run from a directory lists which features are on, and says they came
from `.kubecfgrc.jsonnet`. See [feature gates](../../docs/Advanced-Usage/feature-gates.md).
