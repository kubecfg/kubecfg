local test = import 'test.libsonnet';
local aVar = std.extVar('aVar');
local anVar = std.extVar('anVar');
local filevar = std.extVar('filevar');
local extcode = std.extVar('extcode');
local extVarEnvDefined = std.extVar('extVarEnvDefined');
local extVarEnvUnDefined = std.extVar('extVarEnvUnDefined');

{
  apiVersion: 'v1',
  kind: 'List',
  items: [
    test {
      string: 'bar',
      notAVal: aVar,
      notAnotherVal: anVar,
      filevar: filevar,
      extcode: extcode,
      extVarEnvDefined: extVarEnvDefined,
      extVarEnvUnDefined: extVarEnvUnDefined,
    },
  ],
}
