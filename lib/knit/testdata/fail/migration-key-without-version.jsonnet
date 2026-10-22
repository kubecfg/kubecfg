// expect: migrations.policy/PodDisruptionBudget: expected group/version/kind
local knit = import 'knit/knit.libsonnet';
knit.render(knit.registry { migrations+: { 'policy/PodDisruptionBudget': function(o) o } }, [])
