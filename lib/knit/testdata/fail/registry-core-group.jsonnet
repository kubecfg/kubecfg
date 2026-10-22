// expect: groups.core: no API group core
local knit = import 'knit/knit.libsonnet';
knit.render(knit.registry { groups+: { core: { metadata+: { labels+: { x: 'y' } } } } }, [])
