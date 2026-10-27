// expect: migrations.policy/PodDisruptionBudget: expected group/version/kind
local knit = (import 'kubecfg.libsonnet').knit;
knit.render(knit.registry { migrations+: { 'policy/PodDisruptionBudget': function(o) o } }, [])
