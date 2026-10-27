// expect: types.Container and types.io.k8s.api.core.v1.Container name the same type
local knit = import 'knit/knit.libsonnet';
local reg = knit.registry { types+: {
  Container: { imagePullPolicy: 'Always' },
  'io.k8s.api.core.v1.Container': { terminationMessagePolicy: 'File' },
} };
knit.render(reg, [])
