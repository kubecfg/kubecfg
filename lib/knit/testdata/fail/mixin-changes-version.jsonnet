// expect: the mixin changes autoscaling/HorizontalPodAutoscaler:web to apiVersion autoscaling/v2; change it in a data layer
local knit = import 'knit/knit.libsonnet';
local hpa(v) = { apiVersion: 'autoscaling/' + v, kind: 'HorizontalPodAutoscaler', metadata: { name: 'web' } };
knit.render(knit.registry, [
  hpa('v1'),
  knit.mixin({ [knit.id('autoscaling/HorizontalPodAutoscaler', 'web')]+: { apiVersion: 'autoscaling/v2' } }),
])
