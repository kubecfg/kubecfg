// Copyright 2026 The kubecfg authors
//
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/kubernetes-sigs/kro/pkg/simpleschema"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	crdvalidation "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/validation"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/listtype"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utiljson "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/validation/field"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
)

// validateSimpleSchema checks obj against a kro SimpleSchema
// (https://kro.run/api/specifications/simple-schema), with the API server's own code, in the order it
// checks the spec of a kro instance: unknown fields are errors, as with kubectl's strict field
// validation; defaults are filled in; then the OpenAPI schema and the CEL rules of validation=
// markers are checked. types holds the custom types, as in the types: of a kro schema. The schema
// itself is checked first, as the API server checks the CRD that kro makes of it.
func validateSimpleSchema(obj any, schema, types map[string]any) error {
	s, validator, err := compileSimpleSchema(schema, types)
	if err != nil {
		return fmt.Errorf("invalid SimpleSchema: %w", err)
	}

	// Jsonnet numbers are all float64. Decoding them as the API server does makes whole ones
	// int64, so they pass as integers. It's also the copy that defaulting changes.
	var v any
	if err := roundTrip(obj, &v, utiljson.Unmarshal); err != nil {
		return err
	}

	// obj is the spec of a kro instance. At the top of a resource, apiVersion, kind and metadata
	// would be dropped without an error.
	var errs []string
	for _, p := range pruning.PruneWithOptions(v, s, false, structuralschema.UnknownFieldPathOptions{TrackUnknownFieldPaths: true, ParentPath: []string{"spec"}}) {
		errs = append(errs, fmt.Sprintf("%s: unknown field", strings.TrimPrefix(p, "spec.")))
	}
	// A null where the schema doesn't allow one is dropped, or takes the field's default.
	defaulting.PruneNonNullableNullsWithoutDefaults(v, s)
	defaulting.Default(v, s)
	fieldErrs := validation.ValidateCustomResource(nil, v, validator)
	if m, ok := v.(map[string]any); ok {
		fieldErrs = append(fieldErrs, listtype.ValidateListSetsAndMaps(nil, s, m)...)
	}
	if c := cel.NewValidator(s, false, celconfig.PerCallLimit); c != nil {
		// As in the API server, an object with errors like these could break the rules.
		if slices.ContainsFunc(fieldErrs, func(e *field.Error) bool {
			return slices.Contains([]field.ErrorType{field.ErrorTypeNotSupported, field.ErrorTypeRequired, field.ErrorTypeTooLong, field.ErrorTypeTooMany, field.ErrorTypeTypeInvalid}, e.Type)
		}) {
			errs = append(errs, "(top): the validation= rules were not checked, because of the other errors")
		} else {
			celErrs, _ := c.Validate(context.Background(), nil, s, v, nil, celconfig.RuntimeCELCostBudget)
			fieldErrs = append(fieldErrs, celErrs...)
		}
	}
	for _, e := range fieldErrs {
		if e.Field == "<nil>" {
			e.Field = "(top)"
		}
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		slices.Sort(errs)
		return fmt.Errorf("object is invalid against the SimpleSchema:\n- %s", strings.Join(errs, "\n- "))
	}
	return nil
}

func compileSimpleSchema(schema, types map[string]any) (*structuralschema.Structural, validation.SchemaValidator, error) {
	props, err := simpleschema.ToOpenAPISpec(schema, types)
	if err != nil {
		return nil, nil, err
	}
	// kro sends the schema to the API server as JSON. The round trip sets what only decoding
	// sets, like additionalProperties' Allows for map[string]T.
	var wire extv1.JSONSchemaProps
	if err := roundTrip(props, &wire, json.Unmarshal); err != nil {
		return nil, nil, err
	}
	showRules(&wire)
	var internal apiextensions.JSONSchemaProps
	if err := extv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(&wire, &internal, nil); err != nil {
		return nil, nil, err
	}
	if err := checkSchema(&internal); err != nil {
		return nil, nil, err
	}
	s, err := structuralschema.NewStructural(&internal)
	if err != nil {
		return nil, nil, err
	}
	validator, _, err := validation.NewSchemaValidator(&internal)
	return s, validator, err
}

// checkSchema checks a schema as the API server checks the CRD that kro makes of it, with the
// schema as its spec: list map keys must be fields, defaults must follow the schema, every CEL
// rule must compile, and so on. Checking an object only reaches the parts of the schema that the
// object has.
func checkSchema(spec *apiextensions.JSONSchemaProps) error {
	const group, plural = "kubecfg.io", "instances"
	crd := &apiextensions.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: plural + "." + group},
		Spec: apiextensions.CustomResourceDefinitionSpec{
			Group:    group,
			Names:    apiextensions.CustomResourceDefinitionNames{Plural: plural, Singular: "instance", Kind: "Instance", ListKind: "InstanceList"},
			Scope:    apiextensions.NamespaceScoped,
			Versions: []apiextensions.CustomResourceDefinitionVersion{{Name: "v1", Served: true, Storage: true}},
			Validation: &apiextensions.CustomResourceValidation{OpenAPIV3Schema: &apiextensions.JSONSchemaProps{
				Type:       "object",
				Properties: map[string]apiextensions.JSONSchemaProps{"spec": *spec},
			}},
			PreserveUnknownFields: new(false),
			Conversion:            &apiextensions.CustomResourceConversion{Strategy: apiextensions.NoneConverter},
		},
		Status: apiextensions.CustomResourceDefinitionStatus{StoredVersions: []string{"v1"}},
	}
	var errs []string
	for _, e := range crdvalidation.ValidateCustomResourceDefinition(context.Background(), crd) {
		// The path in the schema, from its top.
		e.Field = strings.TrimPrefix(strings.TrimPrefix(e.Field, "spec.validation.openAPIV3Schema.properties[spec]"), ".")
		if e.Field == "" {
			e.Field = "(top)"
		}
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		slices.Sort(errs)
		return fmt.Errorf("the API server would reject the CRD that kro makes of it:\n- %s", strings.Join(errs, "\n- "))
	}
	return nil
}

func roundTrip(in, out any, unmarshal func([]byte, any) error) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return unmarshal(data, out)
}

// showRules drops the message kro gives every validation= marker, "validation failed", so the
// error names the rule that failed instead. A rule with a line break keeps it: the API server
// wants a message there.
func showRules(s *extv1.JSONSchemaProps) {
	for i := range s.XValidations {
		if s.XValidations[i].Message == "validation failed" && !strings.Contains(s.XValidations[i].Rule, "\n") {
			s.XValidations[i].Message = ""
		}
	}
	for k, p := range s.Properties {
		showRules(&p)
		s.Properties[k] = p
	}
	if s.Items != nil && s.Items.Schema != nil {
		showRules(s.Items.Schema)
	}
	if s.AdditionalProperties != nil && s.AdditionalProperties.Schema != nil {
		showRules(s.AdditionalProperties.Schema)
	}
}
