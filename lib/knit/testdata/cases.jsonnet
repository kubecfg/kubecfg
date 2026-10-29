// Edge cases from the proposal. Run with ./test.sh.
local knit = import 'knit/knit.libsonnet';

local deploy(spec, md={}) = {
  apiVersion: 'apps/v1',
  kind: 'Deployment',
  metadata: { name: 'web' } + md,
  spec: { template: { spec: spec } },
};
local podSpec(out) = out[0].spec.template.spec;
local render(layers, registry=knit.registry) = knit.render(registry, layers);
local hpa(v, name='web') = { apiVersion: 'autoscaling/' + v, kind: 'HorizontalPodAutoscaler', metadata: { name: name } };
local hpaRegistry = knit.registry {
  kinds+: {
    'autoscaling/v1/HorizontalPodAutoscaler': { metadata+: { annotations+: { v1: 'yes' } } },
    'autoscaling/v2/HorizontalPodAutoscaler': { metadata+: { annotations+: { v2: 'yes' } } },
  },
};

local cases = {
  initContainerOrderKept:
    local out = render([deploy({ initContainers: [{ name: 'zz-migrate', image: 'a' }, { name: 'aa-wait', image: 'b' }] })]);
    [c.name for c in podSpec(out).initContainers] == ['zz-migrate', 'aa-wait'],

  envOrderKeptAndOverriddenInPlace:
    local out = render([
      deploy({ containers: [{ name: 'web', env: [{ name: 'B', value: 'b' }, { name: 'A', value: '$(B)' }] }] }),
      deploy({ containers: [{ name: 'web', env: [{ name: 'C', value: 'c' }, { name: 'B', value: 'b2' }] }] }),
    ]);
    podSpec(out).containers[0].env == [{ name: 'B', value: 'b2' }, { name: 'A', value: '$(B)' }, { name: 'C', value: 'c' }],

  containerPortDefaultedProtocolMatches:
    local out = render([
      deploy({ containers: [{ name: 'web', ports: [{ containerPort: 80 }, { containerPort: 53, protocol: 'UDP' }] }] }),
      deploy({ containers: [{ name: 'web', ports: [{ containerPort: 80, protocol: 'TCP', name: 'http' }] }] }),
    ]);
    podSpec(out).containers[0].ports == [
      { containerPort: 80, protocol: 'TCP', name: 'http' },
      { containerPort: 53, protocol: 'UDP' },
    ],

  servicePortsKeyedByPortAndProtocol:
    local svc(ports) = { apiVersion: 'v1', kind: 'Service', metadata: { name: 'dns' }, spec: { ports: ports } };
    local out = render([svc([{ port: 53, protocol: 'UDP', name: 'dns' }]), svc([{ port: 53, name: 'dns-tcp' }])]);
    out[0].spec.ports == [{ port: 53, protocol: 'UDP', name: 'dns' }, { port: 53, name: 'dns-tcp' }],

  volumeMountsKeyedByMountPath:
    local out = render([
      deploy({ containers: [{ name: 'web', volumeMounts: [{ name: 'data', mountPath: '/data' }] }] }),
      deploy({ containers: [{ name: 'web', volumeMounts: [{ name: 'data', mountPath: '/data', readOnly: true }] }] }),
    ]);
    podSpec(out).containers[0].volumeMounts == [{ name: 'data', mountPath: '/data', readOnly: true }],

  atomicListReplaced:
    local out = render([
      deploy({ tolerations: [{ key: 'a', operator: 'Exists' }] }),
      deploy({ tolerations: [{ key: 'b', operator: 'Exists' }] }),
    ]);
    podSpec(out).tolerations == [{ key: 'b', operator: 'Exists' }],

  deleteThenReAddStartsFresh:
    local out = render([
      deploy({ containers: [{ name: 'a', image: 'a', args: ['x'] }, { name: 'b', image: 'b' }] }),
      deploy({ containers: [{ name: 'a', '$patch': 'delete' }] }),
      deploy({ containers: [{ name: 'a', image: 'a2' }] }),
    ]);
    podSpec(out).containers == [{ name: 'b', image: 'b' }, { name: 'a', image: 'a2' }],

  appendViaJsonnet:
    local out = render([
      deploy({ containers: [{ name: 'web', args: ['--port=8080'] }] }),
      deploy({ containers: [{ name: 'web', args: knit.append(['--verbose']) }] }),
    ]);
    podSpec(out).containers[0].args == ['--port=8080', '--verbose'],

  replaceKeyedEntryKeepsPlace:
    local out = render([
      deploy({ containers: [{ name: 'web', image: 'old', env: [{ name: 'A', value: 'a' }] }, { name: 'x', image: 'x' }] }),
      deploy({ containers: [{ name: 'web', '$patch': 'replace', image: 'new' }] }),
    ]);
    podSpec(out).containers == [{ name: 'web', image: 'new' }, { name: 'x', image: 'x' }],

  replaceObjectKeepsPlace:
    local cm(n, d) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: n }, data: d };
    local out = render([[cm('a', { x: '1' }), cm('b', {})], [cm('a', { y: '2' }) + { '$patch': 'replace' }]]);
    [o.metadata.name for o in out] == ['a', 'b'] && out[0].data == { y: '2' },

  typeTemplatesReachAtomicListItems:
    local reg = knit.registry { types+: { ObjectMeta+: { labels+: { 'part-of': 'shop' } } } };
    local sts(claims) = {
      apiVersion: 'apps/v1',
      kind: 'StatefulSet',
      metadata: { name: 'db' },
      spec: { volumeClaimTemplates: claims },
    };
    local out = render([
      sts([{ metadata: { name: 'data' }, spec: { resources: { requests: { storage: '1Gi' } } } }]),
      sts([{ metadata: { name: 'wal' } }]),
    ], reg);
    // Still atomic: the overlay's list replaces the base's.
    out[0].spec.volumeClaimTemplates == [{ metadata: { name: 'wal', labels: { 'part-of': 'shop' } } }],

  versionChangeUsesFinalVersionTemplates:
    local out = render([hpa('v1') + { spec: { minReplicas: 2 } }, hpa('v2')], hpaRegistry);
    out == [hpa('v2') + { metadata+: { annotations: { v2: 'yes' } }, spec: { minReplicas: 2 } }],

  mixinBeforeReplaceDoesNotBlockVersionChange:
    local web = knit.id('autoscaling/HorizontalPodAutoscaler', 'web');
    local out = render([
      hpa('v1'),
      knit.mixin({ [web]+: { spec+: { minReplicas: 5 } } }),
      hpa('v1') + { '$patch': 'replace' },
      hpa('v2'),
    ], hpaRegistry);
    out == [hpa('v2') + { metadata+: { annotations: { v2: 'yes' } } }],

  mixinSnapshotKeepsItsVersionTemplates:
    local web = knit.id('autoscaling/HorizontalPodAutoscaler', 'web');
    local out = render([
      hpa('v1'),
      knit.mixin({
        [knit.id('autoscaling/HorizontalPodAutoscaler', 'backup')]: super[web] + { metadata+: { name: 'backup' } },
      }),
      hpa('v2'),
    ], hpaRegistry);
    std.sort(out, function(o) o.metadata.name) == [
      hpa('v1', 'backup') + { metadata+: { annotations: { v1: 'yes' } } },
      hpa('v2') + { metadata+: { annotations: { v2: 'yes' } } },
    ],

  replaceKeyedList:
    local out = render([
      deploy({ containers: [{ name: 'a', image: 'a' }, { name: 'b', image: 'b' }] }),
      deploy({ containers: knit.replace([{ name: 'c', image: 'c' }]) }),
    ]);
    podSpec(out).containers == [{ name: 'c', image: 'c' }],

  nullAndPatchDeleteRemoveFields:
    local out = render([
      deploy({ containers: [{ name: 'web', image: 'a', resources: { limits: { cpu: '1' } } }], hostNetwork: true }, { creationTimestamp: null }),
      deploy({ containers: [{ name: 'web', resources: knit.delete }], hostNetwork: null }),
    ]);
    // With nothing below it, a null stays a value.
    out[0].metadata == { name: 'web', creationTimestamp: null } && podSpec(out) == { containers: [{ name: 'web', image: 'a' }] },

  replaceMapAndDeleteViaJsonnet:
    local out = render([
      [deploy({ nodeSelector: { a: '1', b: '2' }, containers: [{ name: 'x' }, { name: 'y' }] }), deploy({}, { name: 'gone' })],
      [
        deploy({ nodeSelector: { '$patch': 'replace', c: '3' }, containers: [{ name: 'x' } + knit.delete] }),
        deploy({}, { name: 'gone' }) + knit.delete,
      ],
    ]);
    std.length(out) == 1 && podSpec(out) == { nodeSelector: { c: '3' }, containers: [{ name: 'y' }] },

  finalizersAreASet:
    local cm(f) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'c', finalizers: f } };
    render([cm(['a', 'b']), cm(['c', 'a'])])[0].metadata.finalizers == ['a', 'b', 'c'],

  objectAddedByLaterLayer:
    local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'c' } };
    [o.kind for o in render([deploy({}), cm])] == ['Deployment', 'ConfigMap'],

  vOneListInput:
    local out = render([{ apiVersion: 'v1', kind: 'List', items: [deploy({}), deploy({}, { name: 'api' })] }]);
    [o.metadata.name for o in out] == ['web', 'api'],

  namespaceSkipsClusterScopedKinds:
    local reg = knit.registry { objects+: { metadata+: { namespace: 'shop' } } };
    local cr = { apiVersion: 'rbac.authorization.k8s.io/v1', kind: 'ClusterRole', metadata: { name: 'r' } };
    local out = render([deploy({}), cr], reg);
    out[0].metadata.namespace == 'shop' && !('namespace' in out[1].metadata),

  typeTemplatesReachEveryContainerAndPodMetadata:
    local reg = knit.registry {
      types+: {
        Container+: { env+: [{ name: 'TZ', value: 'UTC' }] },
        ObjectMeta+: { labels+: { 'part-of': 'shop' } },
      },
    };
    local cron = {
      apiVersion: 'batch/v1',
      kind: 'CronJob',
      metadata: { name: 'nightly' },
      spec: { jobTemplate: { metadata: {}, spec: { template: { metadata: {}, spec: {
        initContainers: [{ name: 'init', image: 'i' }],
        containers: [{ name: 'job', image: 'j', env: [{ name: 'TZ', value: 'Europe/Rome' }, { name: 'X', value: 'x' }] }],
      } } } } },
    };
    local o = render([cron], reg)[0], pod = o.spec.jobTemplate.spec.template;
    o.metadata.labels == { 'part-of': 'shop' }
    && o.spec.jobTemplate.metadata.labels == { 'part-of': 'shop' }
    && pod.metadata.labels == { 'part-of': 'shop' }
    && pod.spec.initContainers[0].env == [{ name: 'TZ', value: 'UTC' }]
    // The layer wins over the template; template entries sort first.
    && pod.spec.containers[0].env == [{ name: 'TZ', value: 'Europe/Rome' }, { name: 'X', value: 'x' }],

  templatePrecedence:
    local reg = knit.registry {
      objects+: { metadata+: { annotations+: { a: 'objects', b: 'objects', c: 'objects', d: 'objects' } } },
      groups+: { apps+: { metadata+: { annotations+: { b: 'group', c: 'group', d: 'group' } } } },
      kinds+: {
        'apps/Deployment'+: { metadata+: { annotations+: { c: 'kind', d: 'kind' } } },
        'apps/v1/Deployment'+: { metadata+: { annotations+: { d: 'version' } } },
      },
    };
    render([deploy({})], reg)[0].metadata.annotations == { a: 'objects', b: 'group', c: 'kind', d: 'version' },

  layerBeatsTemplate:
    local reg = knit.registry { objects+: { metadata+: { namespace: 'shop' } } };
    render([deploy({}, { namespace: 'other' })], reg)[0].metadata.namespace == 'other',

  nestedLayersKeepShape:
    local out = render([
      { web: { deployment: deploy({ containers: [{ name: 'web', image: 'a' }] }) }, crds: [deploy({}, { name: 'x' })] },
      [deploy({ containers: [{ name: 'web', image: 'b' }] })],
    ]);
    std.objectFields(out) == ['crds', 'web']
    && out.web.deployment.spec.template.spec.containers == [{ name: 'web', image: 'b' }]
    && [o.metadata.name for o in out.crds] == ['x'],

  unknownKindListsAreAtomic:
    local cr(l) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't' }, spec: { items: l } };
    render([cr([{ name: 'a' }]), cr([{ name: 'b' }])])[0].spec.items == [{ name: 'b' }],

  registrySchemaOverride:
    local reg = knit.registry {
      schema+: { kinds+: { 'example.com/v1/Thing': { fields: { spec: { fields: { items: { list: 'map', keys: ['name'] } } } } } } },
    };
    local cr(l) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't' }, spec: { items: l } };
    render([cr([{ name: 'a' }]), cr([{ name: 'b' }])], reg)[0].spec.items == [{ name: 'a' }, { name: 'b' }],

  generateNameObjectsNeverMatch:
    local job = { apiVersion: 'batch/v1', kind: 'Job', metadata: { generateName: 'migrate-' } };
    std.length(render([job, job])) == 2,

  mixinSeesFinalConfigMap:
    local out = import 'examples/mixin/main.jsonnet';
    out[1].spec.template.metadata.annotations['checksum/config'] == std.md5(std.manifestJson({ DB_HOST: 'db.prod' })),

  // `super` is the stream below the mixin, `self` the final one, wherever
  // render is called.
  mixinSuperAndSelf:
    local cm(v) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'web' }, data: { v: v } };
    local out = { prod: render([
      [cm('base'), deploy({})],
      knit.mixin({
        local stream = self,
        local below = super[knit.id('ConfigMap', 'web')].data.v,
        [knit.id('apps/Deployment', 'web')]+: { metadata+: { annotations+: {
          final: stream[knit.id('ConfigMap', 'web')].data.v,
          below: below,
        } } },
      }),
      [cm('prod')],
    ]) };
    out.prod[1].metadata.annotations == { final: 'prod', below: 'base' },

  // Identity uses the namespace an object ends up with.
  namespaceFromTemplatesMatches:
    local reg = knit.registry { objects+: { metadata+: { namespace: 'shop' } } };
    local out = render([
      deploy({ containers: [{ name: 'web', image: 'a' }] }, { namespace: 'shop' }),
      deploy({}) + { spec+: { replicas: 3 } },
    ], reg);
    std.length(out) == 1 && out[0].spec.replicas == 3 && out[0].metadata.namespace == 'shop'
    && podSpec(out).containers == [{ name: 'web', image: 'a' }],

  clusterScopedIdentityIgnoresNamespace:
    local cr(md) = { apiVersion: 'rbac.authorization.k8s.io/v1', kind: 'ClusterRole', metadata: { name: 'r' } + md };
    std.length(render([cr({ namespace: 'x' }), cr({}) + { rules: [] }])) == 1
    && render([[cr({ namespace: 'x' })], [cr({}) + knit.delete]]) == [],

  scopeFalseIsNamespaced:
    local reg = knit.registry {
      objects+: { metadata+: { namespace: 'shop' } },
      schema+: { clusterScoped+: { 'rbac.authorization.k8s.io/ClusterRole': false } },
    };
    render([{ apiVersion: 'rbac.authorization.k8s.io/v1', kind: 'ClusterRole', metadata: { name: 'r' } }], reg)[0].metadata.namespace == 'shop',

  nullWithNothingBelowIsKept:
    local hr = { apiVersion: 'helm.toolkit.fluxcd.io/v2', kind: 'HelmRelease', metadata: { name: 'h' } };
    local out = render([hr { spec: { values: { controller: { resources: null, image: 'x' } } } }]);
    out[0].spec.values.controller == { resources: null, image: 'x' },

  nullInFreeFormDataIsAValue:
    local crd = {
      apiVersion: 'apiextensions.k8s.io/v1',
      kind: 'CustomResourceDefinition',
      metadata: { name: 'releases.example.com' },
      spec: { group: 'example.com', scope: 'Namespaced', names: { kind: 'Release', plural: 'releases' }, versions: [{
        name: 'v1',
        schema: { openAPIV3Schema: { type: 'object', properties: { spec: { type: 'object', properties: {
          values: { 'x-kubernetes-preserve-unknown-fields': true },
          replicas: { type: 'integer' },
        } } } } },
      }] },
    };
    local rel(spec) = { apiVersion: 'example.com/v1', kind: 'Release', metadata: { name: 'r' }, spec: spec };
    local out = render([[crd, rel({ replicas: 1, values: { a: { x: 1 }, b: 1 } })], [rel({ replicas: null, values: { a: null, b: null } })]]);
    out[1].spec == { values: { a: null, b: null } },

  nullGoesBackToTemplates:
    local reg = knit.registry { objects+: { metadata+: { annotations+: { a: '1' } } } };
    local out = render([deploy({}, { annotations: null }), deploy({}, { annotations: { b: '2' } }), deploy({}, { annotations: null })], reg);
    out[0].metadata.annotations == { a: '1' },

  replaceKeepsTemplates:
    local reg = knit.registry {
      objects+: { metadata+: { namespace: 'shop', labels+: { team: 'platform' } } },
      kinds+: { 'apps/Deployment'+: { spec+: { template+: { spec+: {
        containers+: [{ name: 'web', securityContext: { runAsNonRoot: true } }],
      } } } } },
    };
    local out = render([
      deploy({ containers: [{ name: 'web', image: 'a', args: ['x'] }] }, { labels: { old: 'yes' } }),
      deploy({ containers: [{ name: 'web', '$patch': 'replace', image: 'b' }] }, { '$patch': 'replace', name: 'web', labels: { app: 'web' } }),
    ], reg);
    out[0].metadata == { name: 'web', namespace: 'shop', labels: { team: 'platform', app: 'web' } }
    && podSpec(out).containers == [{ name: 'web', image: 'b', securityContext: { runAsNonRoot: true } }],

  rootReplaceInTemplateDropsLowerTemplates:
    local reg = knit.registry {
      objects+: { metadata+: { labels+: { team: 'platform' } } },
      kinds+: { 'apps/Deployment': { '$patch': 'replace', metadata: { labels: { kind: 'yes' } } } },
    };
    render([deploy({})], reg)[0].metadata.labels == { kind: 'yes' },

  schemaOverrideKeepsObjectMeta:
    local reg = knit.registry {
      types+: { ObjectMeta+: { labels+: { 'part-of': 'shop' } } },
      schema+: { kinds+: { 'example.com/v1/Thing': { fields: { spec: { fields: { items: { list: 'map', keys: ['name'] } } } } } } },
    };
    local thing(f) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't', finalizers: f } };
    render([thing(['x']), thing(['y'])], reg)[0].metadata == { name: 't', finalizers: ['x', 'y'], labels: { 'part-of': 'shop' } },

  mixinCreatedObjectIsLikeADataLayer:
    local reg = knit.registry { objects+: { metadata+: { labels+: { team: 'platform' } } } };
    local base = deploy({ containers: [{ name: 'web', image: 'a' }, { name: 'sidecar', image: 's' }] });
    local over = deploy({ containers: [{ name: 'web', image: 'b' }] });
    local web = knit.id('apps/Deployment', 'web');
    local viaData = render([[base], [over]], reg);
    render([[], knit.mixin({ [web]: base }), [over]], reg) == viaData
    && render([[], knit.mixin({ [web]+: base }), [over]], reg) == viaData,

  crdSchemaFromComposedCrd:
    local versions(keyed) = [{
      name: 'v1',
      schema: { openAPIV3Schema: { type: 'object', properties: { spec: { type: 'object', properties: {
        listeners: { type: 'array', items: { type: 'object', properties: { name: { type: 'string' }, port: { type: 'integer' } } } }
                   + (if keyed then { 'x-kubernetes-list-type': 'map', 'x-kubernetes-list-map-keys': ['name'] } else {}),
      } } } } },
    }];
    local crd(spec) = { apiVersion: 'apiextensions.k8s.io/v1', kind: 'CustomResourceDefinition', metadata: { name: 'gateways.example.com' }, spec: spec };
    local gw(ls) = { apiVersion: 'example.com/v1', kind: 'Gateway', metadata: { name: 'edge' }, spec: { listeners: ls } };
    local out = render([
      [crd({ group: 'example.com', scope: 'Namespaced', names: { kind: 'Gateway', plural: 'gateways' }, versions: versions(false) }), gw([{ name: 'http', port: 80 }, { name: 'https', port: 443 }])],
      [crd({ versions: versions(true) }), gw([{ name: 'https', tls: true }])],
    ]);
    out[1].spec.listeners == [{ name: 'http', port: 80 }, { name: 'https', port: 443, tls: true }],

  mixinDeletesAndKeyedWrites:
    local web = knit.id('apps/Deployment', 'web');
    local cs = [{ name: 'web', image: 'w' }, { name: 'sidecar', image: 's' }];
    local edit(c) = knit.mixin({ [web]+: { spec+: { template+: { spec+: { containers+: c } } } } });
    local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'b' } };
    podSpec(render([[deploy({ containers: cs })], edit({ sidecar: knit.delete })])).containers == [cs[0]]
    && podSpec(render([[deploy({ containers: cs })], edit({ sidecar+: knit.delete })])).containers == [cs[0]]
    && podSpec(render([[deploy({ containers: cs })], edit({ web+: { env+: { FOO: { name: 'FOO', value: '1' } } } })])).containers[0].env == [{ name: 'FOO', value: '1' }]
    && render([[deploy({}), cm], knit.mixin({ [knit.id('ConfigMap', 'b')]: knit.delete })]) == [deploy({})],

  mixinNamespacedId:
    local reg = knit.registry { objects+: { metadata+: { namespace: 'shop' } } };
    local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'web' }, data: { a: '1' } };
    local out = render([[cm], knit.mixin({ [knit.id('ConfigMap', 'web', 'shop')]+: { data+: { b: '2' } } })], reg);
    std.length(out) == 1 && out[0].data == { a: '1', b: '2' },

  mixinEntriesTakeTheMixinsPlace:
    local env(e) = deploy({ containers: [{ name: 'web', env: e }] });
    local out = render([
      env([{ name: 'A', value: 'a' }]),
      knit.mixin({ [knit.id('apps/Deployment', 'web')]+: { spec+: { template+: { spec+: { containers+: { web+: { env+: {
        DB_HOST: { name: 'DB_HOST', value: 'db' },
      } } } } } } } }),
      env([{ name: 'DB_URL', value: 'postgres://$(DB_HOST)' }]),
    ]);
    [e.name for e in podSpec(out).containers[0].env] == ['A', 'DB_HOST', 'DB_URL'],

  mixinCloneInNestedOutput:
    local cm(n) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: n } };
    local out = render([
      { web: cm('web') },
      knit.mixin({ [knit.id('ConfigMap', 'backup')]: super[knit.id('ConfigMap', 'web')] + { metadata+: { name: 'backup' } } }),
    ]);
    out == { web: cm('web'), 'ConfigMap:backup': cm('backup') },

  migrationKeepsDirectives:
    local reg = knit.registry { migrations+: {
      'policy/v1beta1/PodDisruptionBudget': function(o) { apiVersion: 'policy/v1', kind: o.kind, metadata: o.metadata, spec: std.get(o, 'spec', {}) },
    } };
    local pdb = { apiVersion: 'policy/v1beta1', kind: 'PodDisruptionBudget', metadata: { name: 'web' } };
    render([[pdb { spec: { minAvailable: 1 } }], [pdb + knit.delete]], reg) == [],

  versionChangeReplaysAsNewVersion:
    local reg = knit.registry { types+: {
      'io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerSpec': { targetCPUUtilizationPercentage: 80 },
      'io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerSpec': { minReplicas: 2 },
    } };
    render([hpa('v1') + { spec: { maxReplicas: 5 } }, hpa('v2') + { spec: { maxReplicas: 6 } }], reg)[0].spec == { maxReplicas: 6, minReplicas: 2 },

  manyObjectsUnderOneKey:
    local cms = [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'c%d' % i } } for i in std.range(1, 600)];
    std.length(render([{ crds: cms }]).crds) == 600
    && std.length(std.objectFields(render([{ app: { ['c%d' % i]: cms[i - 1] for i in std.range(1, 600) } }]).app)) == 600,

  selfReferentialTypeTemplate:
    // `not` is a JSONSchemaProps too; inside the template it gets none.
    local reg = knit.registry { types+: { JSONSchemaProps+: { description: 'x', not: { description: 'n' } } } };
    local crd = { apiVersion: 'apiextensions.k8s.io/v1', kind: 'CustomResourceDefinition', metadata: { name: 'things.example.com' }, spec: {
      group: 'example.com', names: { kind: 'Thing' }, versions: [{ name: 'v1', schema: { openAPIV3Schema: { type: 'object', properties: { a: { type: 'string' } } } } }],
    } };
    local t = { description: 'x', not: { description: 'n' } };
    render([crd], reg)[0].spec.versions[0].schema.openAPIV3Schema == t { type: 'object', properties: { a: t { type: 'string' } } },

  // A CRD's default for a list key is its author's, even a zero value:
  // {name: a} and {name: a, enabled: false} are one entry.
  crdZeroKeyDefault:
    local crd = { apiVersion: 'apiextensions.k8s.io/v1', kind: 'CustomResourceDefinition', metadata: { name: 'things.example.com' }, spec: {
      group: 'example.com', names: { kind: 'Thing' }, versions: [{ name: 'v1', schema: { openAPIV3Schema: { type: 'object', properties: { spec: { type: 'object', properties: { items: {
        type: 'array',
        'x-kubernetes-list-type': 'map',
        'x-kubernetes-list-map-keys': ['name', 'enabled'],
        items: { type: 'object', required: ['name'], properties: { name: { type: 'string' }, enabled: { type: 'boolean', default: false }, value: { type: 'integer' } } },
      } } } } } } }],
    } };
    local thing(items) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't' }, spec: { items: items } };
    render([[crd, thing([{ name: 'a', value: 1 }])], [thing([{ name: 'a', enabled: false, value: 2 }])]])[1].spec.items
    == [{ name: 'a', enabled: false, value: 2 }],

  // A kind whose registry def has no metadata still gets ObjectMeta's.
  namedKindDefGetsObjectMeta:
    local reg = knit.registry { schema+: { defs+: { 'my.Thing': { fields: { spec: {} } } }, kinds+: { 'example.com/v1/Thing': 'my.Thing' } } };
    local thing(f) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't', finalizers: f } };
    render([[thing(['a'])], [thing(['b'])]], reg)[0].metadata.finalizers == ['a', 'b'],

  // ... even when the def has a metadata field of its own.
  namedKindDefMetadataIsObjectMeta:
    local reg = knit.registry { schema+: { defs+: { 'my.Thing': { fields: { metadata: {} } } }, kinds+: { 'example.com/v1/Thing': 'my.Thing' } } };
    local thing(f) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't', finalizers: f } };
    render([[thing(['a'])], [thing(['b'])]], reg)[0].metadata.finalizers == ['a', 'b'],

  // A real field named like a keyed list's view wins over the view.
  viewKeepsRealUnderscoreField:
    local d = deploy({ containers: [{ name: 'web' }], containers_: 'real' });
    local named = (knit { documents: [{ name: 'a', plus: false, doc: d }] });
    named['$named']('a', function() null).spec.template.spec.containers_ == 'real'
    && named.manifests == [] && named['$named']('a', function() null).spec.template.spec.containers == [{ name: 'web' }],

  // An object a mixin deleted starts again when a layer adds it: the mixin
  // before doesn't block a later version change.
  mixinDeleteStartsOver:
    local web = knit.id('autoscaling/HorizontalPodAutoscaler', 'web');
    render([hpa('v1') + { spec: { minReplicas: 7 } }, knit.mixin({ [web]: knit.delete }), hpa('v1'), hpa('v2')], hpaRegistry)
    == [hpa('v2') + { metadata+: { annotations: { v2: 'yes' } } }],

  // Only the core v1 List holds items; a List of another group is an object.
  otherListIsAnObject:
    local l = { apiVersion: 'example.com/v1', kind: 'List', metadata: { name: 'l' }, items: [] };
    render([l]) == [l],

  // A set of int-or-string items keeps 80 and '80' apart.
  crdMixedSet:
    local crd = { apiVersion: 'apiextensions.k8s.io/v1', kind: 'CustomResourceDefinition', metadata: { name: 'things.example.com' }, spec: {
      group: 'example.com', names: { kind: 'Thing' }, versions: [{ name: 'v1', schema: { openAPIV3Schema: { type: 'object', properties: { spec: { type: 'object', properties: { ports: {
        type: 'array',
        'x-kubernetes-list-type': 'set',
        items: { 'x-kubernetes-int-or-string': true },
      } } } } } } }],
    } };
    local thing(ps) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't' }, spec: { ports: ps } };
    render([[crd, thing([80])], [thing(['80'])]])[1].spec.ports == [80, '80'],

  mixinKeepsLateBinding:
    local cmd(c) = deploy({ containers: [{ name: 'web', command: c }] });
    local out = render([
      cmd(['old']),
      knit.mixin({ [knit.id('apps/Deployment', 'web')]+: { spec+: { template+: { spec+: { containers+: {
        web+: { image: self.command[0] },
      } } } } } }),
      cmd(['new']),
    ]);
    podSpec(out).containers[0].image == 'new',

  migrationFromRemovedApi:
    local reg = knit.registry { migrations+: { 'extensions/v1beta1/Deployment': function(o) o { apiVersion: 'apps/v1' } } };
    render([deploy({}) + { apiVersion: 'extensions/v1beta1' }, deploy({}) + { spec+: { replicas: 2 } }], reg)
    == [deploy({}) + { spec+: { replicas: 2 } }],

  emptyNamespaceIsItsOwn:
    local reg = knit.registry { objects+: { metadata+: { namespace: 'shop' } } };
    local cm(md, d) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'a' } + md, data: d };
    local out = render([[cm({ namespace: 'shop' }, { x: '1' })], [cm({ namespace: '' }, { y: '2' })]], reg);
    [[o.metadata.namespace, o.data] for o in out] == [['shop', { x: '1' }], ['', { y: '2' }]],

  mixinCreatedObjectTemplatePrecedence:
    local reg = knit.registry {
      types+: { ObjectMeta+: { labels+: { team: 'type' } } },
      objects+: { metadata+: { labels+: { team: 'objects' } } },
    };
    local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'c' } };
    render([[], knit.mixin({ [knit.id('ConfigMap', 'c')]: cm })], reg) == render([[cm]], reg),

  mixinCloneKeepsDeletions:
    local reg = knit.registry { objects+: { metadata+: { labels+: { team: 'platform' } } } };
    local cm(n) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: n } };
    local out = render([
      [cm('a') + { metadata+: { labels: knit.delete } }],
      knit.mixin({ [knit.id('ConfigMap', 'b')]: super[knit.id('ConfigMap', 'a')] + { metadata+: { name: 'b' } } }),
    ], reg);
    out == [cm('a'), cm('b')],

  laterTopLevelManifestInNestedOutput:
    local cm(n) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: n } };
    render([{ a: cm('a') }, [cm('b')]]) == { a: cm('a'), 'ConfigMap:b': cm('b') },

  declaredFieldInFreeFormObjectIsTyped:
    local crd = {
      apiVersion: 'apiextensions.k8s.io/v1',
      kind: 'CustomResourceDefinition',
      metadata: { name: 'things.example.com' },
      spec: { group: 'example.com', names: { kind: 'Thing' }, versions: [{ name: 'v1', schema: { openAPIV3Schema: {
        type: 'object',
        properties: { spec: { type: 'object', 'x-kubernetes-preserve-unknown-fields': true, properties: { replicas: { type: 'integer' } } } },
      } } }] },
    };
    local thing(spec) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't' }, spec: spec };
    render([[crd, thing({ replicas: 1, other: 1 })], [thing({ replicas: null, other: null })]])[1].spec == { other: null },

  laterMixinKeepsEarlierMixinsLateBinding:
    local web = knit.id('apps/Deployment', 'web');
    local out = render([
      deploy({ containers: [{ name: 'web' }] }),
      knit.mixin({ [web]+: { spec+: { template+: { spec+: { containers+: { web+: { image: self.command[0] } } } } } } }),
      knit.mixin({ [web]+: { metadata+: { labels+: { a: 'b' } } } }),
      deploy({ containers: [{ name: 'web', command: ['run'] }] }),
    ]);
    podSpec(out).containers[0].image == 'run',

  typeTemplateKeyFieldsCountForMatching:
    local reg = knit.registry { types+: { ContainerPort+: { protocol: 'UDP' } } };
    local pod(ports) = { apiVersion: 'v1', kind: 'Pod', metadata: { name: 'p' }, spec: { containers: [{ name: 'c', ports: ports }] } };
    local out = render([
      pod([{ containerPort: 53 }]),
      knit.mixin({ [knit.id('Pod', 'p')]+: { metadata+: { labels+: { a: 'b' } } } }),
      pod([{ containerPort: 53, protocol: 'UDP', name: 'dns' }]),
    ], reg);
    out[0].spec.containers[0].ports == [{ containerPort: 53, protocol: 'UDP', name: 'dns' }],

  dottedGroupLookingLikeAVersion:
    local reg = knit.registry { kinds+: { 'v1.example.com/Thing': { metadata+: { labels+: { a: 'b' } } } } };
    render([{ apiVersion: 'v1.example.com/v1', kind: 'Thing', metadata: { name: 't' } }], reg)[0].metadata.labels == { a: 'b' },

  nullGoesBackToANullTemplate:
    local reg = knit.registry { kinds+: { 'example.com/Thing': { spec: { value: null } } } };
    local thing(v) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't' }, spec: { value: v } };
    render([thing(1), thing(null)], reg)[0].spec == { value: null },

  laterMixinDoesNotReadTheStreamBelow:
    local pod = { apiVersion: 'v1', kind: 'Pod', metadata: { name: 'p' } };
    local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'c' }, data: { team: 'a' } };
    local p = knit.id('Pod', 'p');
    local out = render([
      [pod],
      knit.mixin({ local stream = self, [p]+: { metadata+: { labels: { team: stream[knit.id('ConfigMap', 'c')].data.team } } } }),
      knit.mixin({ [p]+: { spec+: { hostNetwork: true } } }),
      [cm],
    ]);
    out[0].metadata.labels == { team: 'a' } && out[0].spec == { hostNetwork: true },

  laterMixinReadsEarlierObjectsLate:
    local pod = { apiVersion: 'v1', kind: 'Pod', metadata: { name: 'p' } };
    local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'c' }, data: { team: 'a' } };
    local p = knit.id('Pod', 'p');
    local out = render([
      [pod],
      knit.mixin({
        local stream = self,
        [p]+: if stream[knit.id('ConfigMap', 'c')].data.team == 'a' then { metadata+: { labels: { team: 'a' } } } else {},
      }),
      knit.mixin({ [p]+: { spec+: { hostNetwork: true } } }),
      [cm],
    ]);
    out[0].metadata.labels == { team: 'a' } && out[0].spec == { hostNetwork: true },

  appendingTypeTemplateAppliesOnce:
    local reg = knit.registry { types+: { Container+: { args: knit.append(['--flag']) } } };
    local out = render([
      deploy({ containers: [{ name: 'web' }] }),
      knit.mixin({ [knit.id('apps/Deployment', 'web')]+: { metadata+: { labels+: { a: 'b' } } } }),
      knit.mixin({ [knit.id('apps/Deployment', 'web')]+: { metadata+: { labels+: { c: 'd' } } } }),
    ], reg);
    podSpec(out).containers == [{ name: 'web', args: ['--flag'] }],

  deletedKeyFieldSurvivesAMixin:
    local pod(ports) = { apiVersion: 'v1', kind: 'Pod', metadata: { name: 'p' }, spec: { containers: [{ name: 'c', ports: ports }] } };
    local out = render([
      pod([{ containerPort: 80, protocol: 'TCP' }]),
      pod([{ containerPort: 80, protocol: null }]),
      knit.mixin({ [knit.id('Pod', 'p')]+: { metadata+: { labels+: { a: 'b' } } } }),
    ]);
    out[0].spec.containers[0].ports == [{ containerPort: 80 }],

  mixinAtomicListItemsGetTypeTemplates:
    local reg = knit.registry { types+: { ObjectMeta+: { labels+: { 'part-of': 'shop' } } } };
    local sts = { apiVersion: 'apps/v1', kind: 'StatefulSet', metadata: { name: 'db' }, spec: {} };
    local out = render([
      sts,
      knit.mixin({ [knit.id('apps/StatefulSet', 'db')]+: { spec+: { volumeClaimTemplates: [{ metadata: { name: 'data' } }] } } }),
    ], reg);
    out[0].spec.volumeClaimTemplates == [{ metadata: { name: 'data', labels: { 'part-of': 'shop' } } }],

  mixinLeavesTemplateValuesAlone:
    // `not` is created by the JSONSchemaProps template, which does not
    // apply inside itself; a mixin must not apply it there either.
    local reg = knit.registry { types+: { JSONSchemaProps+: { not: { description: 'n' } } } };
    local crd = { apiVersion: 'apiextensions.k8s.io/v1', kind: 'CustomResourceDefinition', metadata: { name: 'things.example.com' }, spec: {
      group: 'example.com', names: { kind: 'Thing' }, versions: [{ name: 'v1', schema: { openAPIV3Schema: { type: 'object' } } }],
    } };
    local touch(l) = knit.mixin({ [knit.id('apiextensions.k8s.io/CustomResourceDefinition', 'things.example.com')]+: { metadata+: { labels+: { [l]: 'x' } } } });
    render([crd, touch('a'), touch('b')], reg)[0].spec.versions[0].schema.openAPIV3Schema == { type: 'object', not: { description: 'n' } },

  nullKeyFieldTakesWhatIsBelow:
    local reg = knit.registry { types+: { ContainerPort+: { protocol: 'UDP' } } };
    local pod(ports) = { apiVersion: 'v1', kind: 'Pod', metadata: { name: 'p' }, spec: { containers: [{ name: 'c', ports: ports }] } };
    render([pod([{ containerPort: 53 }]), pod([{ containerPort: 53, protocol: null, name: 'dns' }])], reg)[0].spec.containers[0].ports
    == [{ containerPort: 53, protocol: 'UDP', name: 'dns' }],

  typedAdditionalPropertiesInFreeFormObject:
    local crd = {
      apiVersion: 'apiextensions.k8s.io/v1',
      kind: 'CustomResourceDefinition',
      metadata: { name: 'things.example.com' },
      spec: { group: 'example.com', names: { kind: 'Thing' }, versions: [{ name: 'v1', schema: { openAPIV3Schema: {
        type: 'object',
        properties: { spec: { type: 'object', 'x-kubernetes-preserve-unknown-fields': true, additionalProperties: { type: 'integer' } } },
      } } }] },
    };
    local thing(spec) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't' }, spec: spec };
    render([[crd, thing({ x: 1, y: 2 })], [thing({ x: null })]])[1].spec == { y: 2 },

  mixinEntryCopiesSitWithTheirSource:
    // A copy keeps its source's place (ties go by key); new entries go after.
    local web = knit.id('apps/Deployment', 'web');
    local out = render([
      deploy({ containers: [{ name: 'web', image: 'w' }, { name: 'z', image: 'z' }] }),
      knit.mixin({ [web]+: { spec+: { template+: { spec+: { containers+: {
        side: super.web + { name: 'side' },
        extra: { name: 'extra' },
      } } } } } }),
    ]);
    [c.name for c in podSpec(out).containers] == ['side', 'web', 'z', 'extra'],

  schemaLessAtomicEntriesAreNormalized:
    local cr(items) = { apiVersion: 'example.com/v1', kind: 'Thing', metadata: { name: 't' }, spec: { items: items } };
    render([cr([{ name: 'a', z: null, sub: { '$patch': 'delete' } }])])[0].spec.items == [{ name: 'a', z: null }],
};

local failed = [k for k in std.objectFields(cases) if !cases[k]];
assert failed == [] : 'failed: ' + std.join(', ', failed);
std.objectFields(cases)
