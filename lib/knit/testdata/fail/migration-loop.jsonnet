// expect: migrations loop: example.com/v1/Thing -> example.com/v2/Thing -> example.com/v1/Thing
local knit = (import 'kubecfg.libsonnet').knit;
local reg = knit.registry { migrations+: {
  'example.com/v1/Thing': function(o) o { apiVersion: 'example.com/v2' },
  'example.com/v2/Thing': function(o) o { apiVersion: 'example.com/v1' },
} };
knit.render(reg, [{ apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't' } }])
