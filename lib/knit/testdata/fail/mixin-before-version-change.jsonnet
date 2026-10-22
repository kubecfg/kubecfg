// expect: move those mixins after layer 2
local knit = import 'knit/knit.libsonnet';
local hpa(v) = { apiVersion: 'autoscaling/' + v, kind: 'HorizontalPodAutoscaler', metadata: { name: 'web' } };
knit.render(knit.registry, [
  hpa('v1'),
  knit.mixin({ [knit.id('autoscaling/HorizontalPodAutoscaler', 'web')]+: { metadata+: { labels+: { a: 'b' } } } }),
  hpa('v2'),
])
