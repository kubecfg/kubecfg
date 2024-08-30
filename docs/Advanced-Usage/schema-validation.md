# Json Schema Validation

**Supported**: `from v0.31.0`

The `validateJSONSchema` function allows the validation of Jsonnet to a particular [JSONSchema](https://json-schema.org/).

This takes the format of `validateJSONSchema(object, schema)` and will return a runtime error when the provided object
does not uphold the schema.

## Example

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

kubecfg eval --alpha example.jsonnet -o json
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
kubecfg eval --alpha example.jsonnet -o json
ERROR RUNTIME ERROR: object is invalid against the schema: jsonschema: '/age' does not validate with file:///properties/age/minimum: must be >= 0 but found -1
```

