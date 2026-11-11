# Schema validation

kubecfg can check that an object has the shape a schema describes, with a
[JSON Schema](#json-schema) or with kro's [SimpleSchema](#kro-simpleschema). Both functions return
`true` or fail with what's wrong, so they go in an `assert`.

## JSON Schema

**Supported**: `from v0.31.0`

The `validateJSONSchema` function allows the validation of Jsonnet to a particular [JSONSchema](https://json-schema.org/).

This takes the format of `validateJSONSchema(object, schema)` and will return a runtime error when the provided object
does not uphold the schema.

### Example

Using this example `schema.json` file:

```json
{
   "properties": {
      "age": {
         "description": "Age in years which must be equal to or greater than zero.",
         "minimum": 0,
         "type": "integer"
      },
      "firstName": {
         "description": "The person's first name.",
         "type": "string"
      },
      "lastName": {
         "description": "The person's last name.",
         "type": "string"
      }
   },
   "type": "object"
}
```

We can use this schema to validate objects which are placed within our Jsonnet code.

Take the following `example.jsonnet` file:

```jsonnet
local kubecfg = import 'kubecfg.libsonnet';
{
  assert kubecfg.validateJSONSchema($.person, import 'schema.json'),

  person: {
    age: 26,
  },
}
```

This can be successfully evaluated, as it is valid against the schema:

```sh

kubecfg eval --features=eval example.jsonnet -o json
{
   "person": {
      "age": 26
   }
}
```

Note that `firstName` and `lastName` are not `required` items, hence this object remains valid
when they are omitted.

By modifying the `person.age` value to `-1` we can force this to produce an error,
as this becomes invalid against our schema:

```sh
kubecfg eval --features=eval example.jsonnet -o json
ERROR RUNTIME ERROR: object is invalid against the schema: jsonschema validation failed with 'file:///schema.json#'
- at '/age': minimum: got -1, want 0
```

## kro SimpleSchema

> [!WARNING]
> **Alpha.** Turn it on with `--features=simpleschema` (see [Feature gates](feature-gates.md)). It
> can change in any release.

[kro](https://kro.run) describes the fields of an API with
[SimpleSchema](https://kro.run/api/specifications/simple-schema): one line per field, with its type
and its rules. It's short, and it's YAML. This `app-schema.yaml` is laid out as the `schema:` of a
kro ResourceGraphDefinition: `spec` has the fields, `types` the types it defines.

```yaml
spec:
  image: string | required=true validation="!self.endsWith(':latest')"
  replicas: integer | required=true minimum=1 maximum=10
  args: "[]string"
  env: "map[string]string"
  ports: "[]Port | listType=map listMapKey=port"
types:
  Port:
    name: string | required=true maxLength=15
    port: integer | required=true minimum=1 maximum=65535
```

`kubecfg.validateSimpleSchema(obj, schema, types={})` checks `obj` against it:

```jsonnet
local kubecfg = import 'kubecfg.libsonnet';
{
  appSchema:: import 'yaml:app-schema.yaml',
  settings:: { image: 'nginx:latest', replicas: 0, prots: [] },

  assert kubecfg.validateSimpleSchema(self.settings, self.appSchema.spec, self.appSchema.types),

  deployment: { /* made from $.settings */ },
}
```

```console
$ kubecfg show --features=simpleschema app.jsonnet
error reading app.jsonnet: RUNTIME ERROR: object is invalid against the SimpleSchema:
- image: Invalid value: "nginx:latest": failed rule: !self.endsWith(':latest')
- prots: unknown field
- replicas: Invalid value: 0: replicas in body should be greater than or equal to 1
```

The schema and the types are plain values. They can be written in Jsonnet or YAKL, imported from
YAML, or read from a ResourceGraphDefinition, as `rgd.spec.schema.spec` and
`std.get(rgd.spec.schema, 'types', {})` (`types:` is optional). A `null` schema or `null` types
are empty, as in kro.

### What is checked

kro's code turns the schema into an OpenAPI schema, as kro does when it makes a CRD. The
Kubernetes API server's code then checks the schema whole, as it checks kro's CRD: a
`listMapKey` must be a field, every `validation=` rule must compile, and so on, also for fields
that `obj` doesn't have. Then it checks the object, in the order the API server checks the spec
of a kro instance:

1. A field the schema doesn't have is an error, as `kubectl apply` reports it.
2. Defaults (`default=`) are filled in, also for a field that is `null`. A `null` field without
   a default is dropped. That's only for the checks: `obj` itself doesn't change.
3. The types and the markers: `required`, `minimum`, `pattern`, `enum`, `listType`, ...
4. The CEL rules of `validation=` markers, with `self` the field. As in the API server, they
   don't run while there's a missing field or a value of the wrong type.

`immutable=true` compares an object with its old version, and there's none here, so it never fails.

### A base checks its overlays

An `assert` checks the object as the overlays leave it: in it, `self` is the final object. So a
base can hold the schema and the asserts, and overlays only change the settings:

```jsonnet
// prod.jsonnet
(import 'base.jsonnet') + {
  settings+: { web+: { replicas: 0 } },
}
```

```console
$ kubecfg show --features=simpleschema prod.jsonnet
error reading prod.jsonnet: RUNTIME ERROR: object is invalid against the SimpleSchema:
- replicas: Invalid value: 0: replicas in body should be greater than or equal to 1
	internal:///kubecfg.libsonnet:103:49-103	function <anonymous>
	file:///.../base.jsonnet:51:10-100	object <anonymous>
	Checking object assertions
```

The line of the base names the assert that failed. An overlay can change the schema too, with
`appSchema+: { spec+: { ... } }`.

In YAKL, the assert is a [`${assert ...}` key](yakl.md#asserts):

```yaml
^^:: web
${assert kubecfg.validateSimpleSchema(self, $.appSchema.spec, $.appSchema.types)}:
image: nginx:1.27
replicas: 2
```

[`examples/experimental/simpleschema`](https://github.com/kubecfg/kubecfg/tree/main/examples/experimental/simpleschema)
has a base and a prod overlay, in Jsonnet and in YAKL.
