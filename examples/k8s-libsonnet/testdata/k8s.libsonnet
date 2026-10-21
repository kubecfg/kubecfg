// The k8s jsonnet library helps you write k8s objects.
//
// It combines three parts in one file (in order to reduce the number of library imports)
//
// 1. a selection of helper functions to map, extend, convert objects in ways that are useful for manipulating k8s objects.
//
// 2. a system to build "component" and "mount" them in parent components.
//
// 3. a prototype-based k8s "schema".
local kubecfg = import 'kubecfg.libsonnet';
{
  // -------------------------
  // generic helpers
  // -------------------------

  // workaround for https://github.com/google/go-jsonnet/issues/736
  // local objectHas(o, f) = std.objectHas(o, f),
  local objectHas(o, f) = std.setMember(f, std.objectFields(o)),

  // applies overlay to each field of obj.
  applyOverlayEach(obj, overlay):: $.mapObject(function(v) v + overlay, obj),

  // like applyOverlayEach but with the extension order flipped: for each field
  // return "base + field".
  deriveEach(base, obj):: $.mapObject(function(v) base + v, obj),

  // mapObject applies f to all the values of the o object. Unlike std.mapWithKey, the "hidden" property of the fields is preserved.
  mapObject(f, o):: std.foldl(function(acc, i) acc + (
    local v = f(o[i]);
    if objectHas(o, i) then { [i]: v } else { [i]:: v }
  ), std.objectFieldsAll(o), {}),

  // Like std.objectValues but adds a `[key`] field with the key of each element.
  //
  // if key is `null`, this function behaves like `std.objectValues`
  asKeyedArray(key, obj):: [
    obj[k] { [key]: k }
    for k in std.objectFields(obj)
  ],

  // special case of asKeyedArray that uses key name `name`, which is very common in the k8s schemas.
  asNamedArray(obj):: self.asKeyedArray('name', obj),

  // -------------------------
  // components
  // -------------------------

  // The "component" pattern is a way to factor out deployment configs for a component (subtree)
  // into another file.
  //
  // ```
  // app.jsonnet:
  // {
  //   k8s:: import "k8s.libsonnet",
  //   components_:: {
  //      foo: import "foo.libsonnet",
  //   },
  //   components: $.k8s.mountComponents($, $.components_),
  // }
  //
  // foo.libsonnet:
  // k8s.Component {
  //   service: $.k8s.v1.Serivce {
  //      // ...
  //   },
  // }
  // ```
  //
  // Often times you need to propagate some ambient parameters into the mounted components.
  // A common example is when you want that the mounted component inherits the namespace of the parent.
  //
  // Every extension of the `$.k8s` object will also applied to the `$.k8s` object in the mounted component.
  //
  // ```jsonnet
  // app.jsonnet:
  // {
  //   k8s:: (import "k8s.libsonnet") {
  //     ObjectMeta+: {
  //       namespace: 'myns',
  //     },
  //   },
  //   components_:: {
  //      foo: import "foo.libsonnet",
  //   },
  //   components: $.k8s.mountComponents($, $.components_),
  // }
  //
  // foo.libsonnet:
  // k8s.Component {
  //   service: $.k8s.v1.Service {
  //     metadata+: { name: 'foo' },
  //     // ...
  //   },
  // }
  // ```
  //
  // Will render:
  //
  // ```yaml
  // apiVersion: v1
  // kind: Service
  // metadata:
  //   name: foo
  //   namespace: myns
  // ```
  //
  // Subcomponents can further override the k8s templates:
  // ```
  // app.jsonnet:
  // {
  //   k8s:: (import "k8s.libsonnet") {
  //     ObjectMeta+: {
  //       namespace: 'myns',
  //     },
  //   },
  //   components_:: {
  //      foo: import "foo.libsonnet",
  //   },
  //   components: $.k8s.mountComponents($, $.components_),
  // }
  //
  // foo.libsonnet:
  // k8s.Component {
  //   k8s+:: {
  //     // tip: by using ObjectTemplateMeta the label will also be used in pod spec templates.
  //     ObjectTemplateMeta+: {
  //       labels+: {
  //         'idpe.influxdata.io/component': 'foo',
  //       },
  //     },
  //   },
  //   service: $.k8s.v1.Service {
  //      // ...
  //   },
  // }
  // ```
  //
  // Not everything can be achieved by just overriding the standard k8s resource templates.
  // For everything else we have the good old `conf` and `envConf` objects, which are weaved through
  // in mounted components. Example:
  //
  // ```jsonnet
  // app.jsonnet:
  // {
  //   k8s:: import "k8s.libsonnet",
  //   envConf: ....
  //   components_:: {
  //      foo: import "foo.libsonnet",
  //   },
  //   components: $.k8s.mountComponents($, $.components_),
  // }
  //
  // foo.libsonnet:
  // k8s.Component {
  //   bar: somelib.MakeBar($.envConf),
  // }
  // ```
  //
  // You may not want to instantiate a component just yet, but you still want to leverage the ambient config
  // propagation of mounted abstract components. If you put your component in a hidden field you can reuse it later:

  // ```jsonnet
  // app.jsonnet:
  // {
  //   k8s:: import "k8s.libsonnet",
  //   envConf: ....
  //   components_:: {
  //      foo:: import "foo.libsonnet",
  //   },
  //   components: $.k8s.mountComponents($, $.components_),
  //
  //   something: $.components.foo {
  //     name: 'something',
  //   }
  //   other: $.components.foo {
  //     name: 'other',
  //   }
  // }
  //
  mountComponents(root, components=root.components_):: $.deriveEach({
    k8s:: $,
    envConf:: std.get(root, 'envConf', {}),
    conf:: std.get(root, 'conf', {}),
    coreConf:: std.get(root, 'coreConf', {}),
    mixins:: std.get(root, 'mixins', {}),
    images:: std.get(root, 'images', {}),
    // TODO(mkm): figure out a way to generalize the plumbing.
  }, components),

  Component:: {
    k8s+:: if 'k8s' in super then super.k8s else $,
    components_:: {},
    components: self.k8s.mountComponents(self, self.components_),
  },

  // Deprecated; always use a Component (kept for backwards compatibility)
  RootComponent:: $.Component {
    k8s:: $,
  },

  // The typedNamedArrays function allows to compactly define:
  // {
  //   containers_:: {},
  //   containers: $.asNamedArray($.deriveEach(Container, self.containers_)),
  //   ....
  // }
  typedNamedArrays(arr):: std.foldl(function(acc, el)
    local name = if std.isArray(el) then el[0] else el;
    local type = if std.isArray(el) then el[1] else {};
    acc {
      [name + '_']+:: {},
      [name]: $.asNamedArray($.deriveEach(type, self[name + '_'])),
    }, arr, { [adoptUpstreamKey]:: adopterFromTypedNamedArrays(arr) }),
  local adoptUpstreamKey = '_adoptUpstream',

  // This function returns the "adopter" function for a given partial type (object subtree).
  // The adopter function logically performs the inverse function of typedNamedArrays.
  local adopterFromPartialType(obj) = std.get(obj, adoptUpstreamKey, function(o) o),

  // returns a function that will convert an upstream object into an object in the style of what users of this library
  // would have crafted, according to the "typed named array" field definitions (containers -> containers_, env -> env_, ...).
  // `arr` is an array of field definitions. A field definition is either string (the name of a field, e.g. 'containers')
  // or in turn an tuple of [field_name, element_type]
  local adopterFromTypedNamedArrays(arr) = function(o) std.foldl(
    function(acc, d) adoptNamedArray(acc, if std.isArray(d) then d else [d, {}]),
    arr,
    o
  ),

  local adoptNamedArray(o, fieldDef) =
    local name = if std.isArray(fieldDef) then fieldDef[0] else fieldDef;
    local type = if std.isArray(fieldDef) then fieldDef[1] else {};
    local items = std.get(o, name, []);
    // elements without a name (e.g. `ports: [{ containerPort: 80 }]`) cannot be keyed; leave the array as-is.
    if !std.all([std.objectHas(i, 'name') for i in items]) then o else
    o {
      [name + '_']+:: {
        [i.name]+: i
        for i in items
      },
      [name]: $.asNamedArray($.mapObject(function(v) adoptObject(v, type), self[name + '_'])),
    },

  // "Objectified" array
  //
  // Basic usage:
  //
  // $.k8s.orray({containers: 'name'}) {
  //  containers_: { foo: { image: 'bar:latest' } }
  // }
  //
  // `orray` produces an object that, for each field in the `fieldDef` parameter,
  // contains a field `f` and a hidden field `f_`. The field `f` contains an array
  // that is derived from `f_` using `asKeyedArray`.
  //
  // TODO(mkm): implement `typedNamedArrays` in terms of `orray`.
  orray(fieldDefs):: std.foldl(function(acc, fieldDef)
    local field = fieldDef[0], key = fieldDef[1];
    {
      [field + '_']+:: {},
      [field]: $.asKeyedArray(key, self[field + '_']),
    }, objectEntries(fieldDefs), {}),


  // a zip of std.objectFields and std.objectValues.
  local objectEntries(o) = [[k, o[k]] for k in std.objectFields(o)],

  // adopt existing resources and make them extend the types defined by the k8s-libsonnet library.
  adopt(objs):: kubecfg.deepMap(function(o) adoptObject(kubecfg.toOverlay(o), $.typeFor(o)), objs),

  local adoptObject(obj, typ) = deepMapObject(adoptObjectSubtree, typ + obj),
  local adoptObjectSubtree(obj) = adopterFromPartialType(obj)(obj),

  typeFor(o)::
    local gv = std.splitLimitR(o.apiVersion, '/', 1),
          group = if std.length(gv) == 2 then std.get($, gv[0], {}) else $,
          version = std.get(group, gv[std.length(gv) - 1], {});
    std.get(version, o.kind, $.Object),

  // deepMapObject(func, o): Apply the given function to each
  // object in nested collection o, preserving the structure of o.
  // It traverses arrays but it doesn't invoke the function on on arrays
  local deepMapObject(func, o) = (
    if std.isObject(o) then
      func($.mapObject(function(o) deepMapObject(func, o), o))
    else if std.isArray(o) then
      [deepMapObject(func, elem) for elem in o]
    else o
  ),
  assert std.assertEqual('%s' % [deepMapObject(std.toString, { a: { b: [{ c: 1 }] } })], '{"a": "{\\"b\\": [\\"{\\\\\\"c\\\\\\": 1}\\"]}"}'),

  // -------------------------
  // types
  // -------------------------
  //
  // This section implements the k8s "schema".
  //
  // If you build  your k8s objects by deriving from "prototype" objects in this library, you get the following benefits:
  //
  // 1. a little bit of validation. We've sprinkled some asserts to help with common errors
  //      or errors that are better reported if caught early, but this is no replacement for kubeval.
  //
  // 2. a consistent set of *extension points*. You often need to apply an overlay to all resources of a given kind
  //    or to the metadata of all resources. The location of some fields depends on the schema. For example,
  //    you often want all objects to have some labels, but some objects are created by controllers and need
  //    to have the metadata duplicated. Prototype objects like ObjectTemplateMeta
  //
  // 3. jsonnet has a very neat object extension mechanism, but it doesn't work when you have arrays. The k8s model
  //    unfortunately uses arrays quite a lot. We have a mechanism to help with that, called `k8s.asNamedArray`,
  //    which lets you write an object and generates an array which will be consumed by k8s.
  //    (example: `foo_: { bar: { ... } }` -> `foo: [ { name: "bar", ... } ]`)
  //    If you build your objects from schema the prototypes you get these conversions for free.
  //
  //    TIP: Always write `foo_:` and not `foo_::`; this way if for some reason `foo_` is not provided by the prototype
  //    you'll notice an error when kubecfg renders the object or when you validate with kubeval, instead of just silently
  //    dropping your content.
  //
  // 4. TODO: make it easier to connect resources that use labels and selectors (services -> pods, deplyment -> pod templates)
  //
  // The structure mirrors the k8s schema, see https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23
  //
  // Apart from the root types `Object`, `ObjectTemplateMeta` and `ObjectMeta`, all other types are organized
  // using the familiar hierarchy you would use when defining the `apiVersion` field.
  //
  // Examples:
  //
  // `v1.Secret` -> `{ apiVersion: 'v1', kind: 'Secret' }`
  // `apps.v1.Deployment` -> `{ apiVersion: 'apps/v1', kind: 'Deployment' }`

  Object:: {
    apiVersion: error 'required apiVersion',
    kind: error 'required kind',
    metadata+: $.ObjectMeta,
  },

  // ObjectTemplateMeta is like template
  ObjectTemplateMeta:: {
  },

  local clusterScoped = { metadata+: { namespace:: null } },

  ObjectMeta:: self.ObjectTemplateMeta {
    assert 'name' in self || 'generateName' in self : 'Object metadata must have a "name" or a "generateName"',
  },

  v1:: {
    local v = self,
    Object: $.Object {
      apiVersion: 'v1',
    },
    Namespace: v.Object + clusterScoped {
      kind: 'Namespace',
    },
    ServiceAccount: v.Object {
      kind: 'ServiceAccount',
    },
    Secret: v.Object {
      kind: 'Secret',
    },
    ConfigMap: v.Object {
      kind: 'ConfigMap',
    },
    Service: v.Object {
      kind: 'Service',
      spec+: v.ServiceSpec,
    },
    ServiceSpec: $.typedNamedArrays(['ports']),

    // a Pod gets the same overlays as every other pod template.
    Pod: v.Object + v.PodTemplateSpec {
      kind: 'Pod',
    },

    PodTemplateSpec: {
      local pts = self,
      local nonSidecarContainers = [i for i in pts.spec.containers if !std.get(i, 'sidecar', false)],
      metadata+: $.ObjectTemplateMeta + if std.length(nonSidecarContainers) == 1 then {
        annotations+: {
          // If a pod contains sidecar containers, it's a good idea to annotate the name of the main container
          // so that tools like `kubectl exec` or `kubectl log` can default to it.
          // When a pod contains only one container then the annotation is not normally needed.
          // Some sidecars (like istio proxy) are dynamically added,
          // so from the POV of the jsonnet there is only one container, but if we force the annotation
          // the singleton container will be recognized as the default container even after the dynamic sidecars
          // have been added.
          'kubectl.kubernetes.io/default-container': nonSidecarContainers[0].name,
        },
      } else {},
      spec+: v.PodSpec,
    },
    PodSpec+: $.typedNamedArrays([['containers', v.Container], ['initContainers', v.Container], 'volumes']),
    Container+: $.typedNamedArrays(['env', ['ports', v.ContainerPort], 'volumeMounts']),
    ContainerPort+: {
      containerPort: error 'must set containerPort',
      port:: '_do_not_use_this',
      assert self.port == '_do_not_use_this' : 'use "containerPort" here, not "port"',
    },
  },

  apps:: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'apps/',
    },
    v1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1',
      },
      Deployment: v.Object {
        kind: 'Deployment',
        spec+: v.DeploymentSpec,
      },
      DeploymentSpec: {
        template+: $.v1.PodTemplateSpec,
      },
      StatefulSet: v.Object {
        kind: 'StatefulSet',
        spec+: v.StatefulSetSpec,
      },
      StatefulSetSpec: {
        template+: $.v1.PodTemplateSpec,
        volumeClaimTemplates_:: {},
        // cannot use `$.typedNamedArrays` or `$.asNamedArray` because this uses `.metadata.name` and not `.name` as the key
        // TODO(mkm): figure out if it's worth to generlize.
        volumeClaimTemplates: local obj = self.volumeClaimTemplates_; [
          obj[k] { metadata+: { name: k } }
          for k in std.objectFields(obj)
        ],
      },
      DaemonSet: v.Object {
        kind: 'DaemonSet',
        spec+: v.DaemonSetSpec,
      },
      DaemonSetSpec: {
        template+: $.v1.PodTemplateSpec,
      },
    },
  },

  batch:: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'batch/',
    },
    v1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1',
      },
      Job: v.Object {
        kind: 'Job',
        spec+: v.JobSpec,
      },
      JobSpec: {
        template+: $.v1.PodTemplateSpec,
      },
      CronJob: v.Object {
        kind: 'CronJob',
        spec+: v.CronJobSpec,
      },
      CronJobSpec: {
        jobTemplate+: { spec+: v.JobSpec },
      },
    },
  },

  policy:: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'policy/',
    },

    v1beta1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1beta1',
      },
      PodDisruptionBudget: v.Object {
        kind: 'PodDisruptionBudget',
      },
    },
    v1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1',
      },
      PodDisruptionBudget: v.Object {
        kind: 'PodDisruptionBudget',
      },
    },
  },

  'networking.k8s.io':: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'networking.k8s.io/',
    },
    v1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1',
      },
      Ingress: v.Object {
        kind: 'Ingress',
      },
    },
  },

  'monitoring.coreos.com':: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'monitoring.coreos.com/',
    },
    v1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1',
      },
      PodMonitor: v.Object {
        kind: 'PodMonitor',
        spec+: v.PodMonitorSpec,
      },
      PodMonitorSpec:: self.CommonMonitorSpec {
        podMetricsEndpoints_:: {},
        // cannot use `$.typedNamedArrays` or `$.asNamedArray` because this uses `.port` and not `.name` as a key.
        // TODO(mkm): figure out if it's worth to generlize.
        podMetricsEndpoints: local obj = self.podMetricsEndpoints_; [
          obj[k] { port: k }
          for k in std.objectFields(obj)
        ],
      },
      ServiceMonitor: v.Object {
        kind: 'ServiceMonitor',
        spec+: v.ServiceMonitorSpec,
      },
      ServiceMonitorSpec:: self.CommonMonitorSpec {
        endpoints_:: {},
        // cannot use `$.typedNamedArrays` or `$.asNamedArray` because this uses `.port` and not `.name` as a key.
        // TODO(mkm): figure out if it's worth to generlize.
        endpoints: local obj = self.endpoints_; [
          obj[k] { port: k }
          for k in std.objectFields(obj)
        ],
      },
      CommonMonitorSpec:: {
        selector_:: {
          matchExpressions_:: {},
        },
        selector: self.selector_ + if std.length(self.selector_.matchExpressions_) == 0 then {} else {
          matchExpressions: local obj = self.matchExpressions_; [
            obj[k] { key: k }
            for k in std.objectFields(obj)
          ],
        },
      },
    },
  },

  'networking.istio.io':: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'networking.istio.io/',
    },
    v1beta1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1beta1',
      },
      VirtualService: v.Object {
        kind: 'VirtualService',
      },
      DestinationRule: v.Object {
        kind: 'DestinationRule',
      },
      EnvoyFilter: v.Object {
        kind: 'EnvoyFilter',
      },
    },
  },

  'bitnami.com':: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'bitnami.com/',
    },
    v1alpha1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1alpha1',
      },
      SealedSecret: v.Object {
        kind: 'SealedSecret',
      },
    },
  },

  'maupu.org':: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'maupu.org/',
    },
    v1beta1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1beta1',
      },
      VaultSecret: v.Object {
        kind: 'VaultSecret',
        spec+: v.VaultSecretSpec,
      },
      VaultSecretSpec:: {
        config: {
          addr: error 'required',
          auth: {
            kubernetes: {
              cluster: error 'required',
              role: 'docker-secrets',
              serviceAccount: 'default',
            },
          },
        },
        secrets_:: {},
        secrets: local obj = self.secrets_; [
          obj[k] { path: k }
          for k in std.objectFields(obj)
        ],
      },
    },
  },

  'kafka.strimzi.io':: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'kafka.strimzi.io/',
    },
    v1beta2:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1beta2',
      },
      KafkaTopic: v.Object {
        kind: 'KafkaTopic',
        spec+: v.KafkaTopicSpec,
      },
      KafkaTopicSpec:: {
        topicName: error 'required',
      },
    },
  },

  'rbac.authorization.k8s.io':: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'rbac.authorization.k8s.io/',
    },
    v1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1',
      },
      Role: v.Object {
        kind: 'Role',
        rules: [],
      },
      RoleBinding: v.Object {
        kind: 'RoleBinding',
        roleRef: v.RoleRef,
        subjects: [],
      },
      RoleRef:: {
        apiGroup: 'rbac.authorization.k8s.io',
        kind: 'Role',
        name: error 'required',
      },
      ClusterRole: v.Object + clusterScoped {
        kind: 'ClusterRole',
        rules: [],
      },
      ClusterRoleBinding: v.Object + clusterScoped {
        kind: 'ClusterRoleBinding',
        roleRef: v.ClusterRoleRef,
        subjects: [],
      },
      ClusterRoleRef:: {
        apiGroup: 'rbac.authorization.k8s.io',
        kind: 'ClusterRole',
        name: error 'required',
      },
      UserRef:: {
        apiGroup: 'rbac.authorization.k8s.io',
        kind: 'User',
        name: error 'required',
      },
      GroupRef:: {
        apiGroup: 'rbac.authorization.k8s.io',
        kind: 'Group',
        name: error 'required',
      },
      ServiceAccountRef:: {
        apiGroup: '',
        kind: 'ServiceAccount',
        namespace: std.get($.ObjectMeta { name:: '' }, 'namespace', error 'required namespace'),
        name: error 'required',
      },
    },
  },

  'apiextensions.k8s.io':: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'apiextensions.k8s.io/',
    },
    v1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1',
      },
      CustomResourceDefinition: v.Object + clusterScoped {
        kind: 'CustomResourceDefinition',
      },
    },
  },

  'cert-manager.io':: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'cert-manager.io/',
    },
    v1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1',
      },
      Certificate: v.Object {
        kind: 'Certificate',
      },
    },
  },

  // Red Hat OpenShift objects
  'route.openshift.io':: {
    local group = self,
    Object:: $.Object {
      apiVersion: 'route.openshift.io/',
    },
    v1:: {
      local v = self,
      Object: group.Object {
        apiVersion+: 'v1',
      },
      Route: v.Object {
        kind: 'Route',
      },
    },
  },
}
