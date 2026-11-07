// expect: knit: $.mixin in a YAKL file
local knit = (import 'kubecfg.libsonnet').knit;
(knit { a: self['$merge'](null, knit.mixin({})) }).a
