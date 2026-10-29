// A vendored chart still ships policy/v1beta1; a migration lifts it to
// policy/v1 before anything else, so a policy/v1 overlay matches it.
local knit = import 'knit/knit.libsonnet';

local registry = knit.registry {
  kinds+: { 'policy/PodDisruptionBudget'+: { metadata+: { labels+: { tier: 'critical' } } } },
  migrations+: {
    'policy/v1beta1/PodDisruptionBudget': function(o) o { apiVersion: 'policy/v1' },
  },
};

knit.render(registry, [
  import 'yaml:base.yaml',
  import 'yaml:prod.yaml',
])
