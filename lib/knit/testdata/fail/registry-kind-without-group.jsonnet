// expect: no kind Deployment in the core group; did you mean apps/Deployment?
local knit = import 'knit/knit.libsonnet';
knit.render(knit.registry { kinds+: { Deployment: { metadata+: { labels+: { x: 'y' } } } } }, [])
