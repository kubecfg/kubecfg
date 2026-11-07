// expect: groups.core: no API group core
local knit = (import 'kubecfg.libsonnet').knit;
knit.render(knit.registry { groups+: { core: { metadata+: { labels+: { x: 'y' } } } } }, [])
