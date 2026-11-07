// expect: no kind Deployment in the core group; did you mean apps/Deployment?
local knit = (import 'kubecfg.libsonnet').knit;
knit.render(knit.registry { kinds+: { Deployment: { metadata+: { labels+: { x: 'y' } } } } }, [])
