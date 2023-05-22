// Copyright 2023 The kubecfg authors
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

// Package yamloc provides a tool that converts a location in a yaml file into a jsonnet "path" into that file.
// Jsonnet paths are similar to JSONPaths.
package yamloc

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Return a JsonnetPath that will locate the yaml field at a given line. Lines are numbered from 1 (like most editors do).
func LineToPath(src []byte, line int) (string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		return "", err
	}
	return lineFinder(line).find(&root)
}

func nodeKind(node *yaml.Node) string {
	switch node.Kind {
	case yaml.DocumentNode:
		return "DocumentNode"
	case yaml.SequenceNode:
		return "SequenceNode"
	case yaml.MappingNode:
		return "MappingNode"
	case yaml.ScalarNode:
		return "ScalarNode"
	case yaml.AliasNode:
		return "AliasNode"
	}
	return "UnknownNode"
}

type lineFinder int

func (line lineFinder) find(root *yaml.Node) (string, error) {
	res, cont, err := lineFinder(line).visit("$", root)
	if cont {
		return "", fmt.Errorf("didn't find AST node for line %d", line)
	}
	return res, err
}

func (line lineFinder) visit(acc string, node *yaml.Node) (string, bool, error) {
	switch node.Kind {
	case yaml.DocumentNode:
		if got, want := len(node.Content), 1; got != want {
			return "", false, fmt.Errorf("document node children asserion failed: got %q, want %q", got, want)
		}
		return line.visit(acc, node.Content[0])
	case yaml.SequenceNode:
		for i, c := range node.Content {
			if res, cont, err := line.visit(fmt.Sprintf("%s[%d]", acc, i), c); !cont || err != nil {
				return res, cont, err
			}
		}
		return acc, true, nil
	case yaml.MappingNode:
		// The contents of a MappingNode is a funny Label,Value,Label,Value,... pair sequence
		// so we have to keep track of the last field value so we can pass it down as accumulator
		// on the next call to visit down the following value node.
		nextAcc := acc
		for _, c := range node.Content {
			if c.Kind == yaml.ScalarNode {
				nextAcc = fmt.Sprintf("%s.%s", acc, c.Value)
				if c.Line == int(line) {
					return nextAcc, false, nil
				}
			} else {
				if res, cont, err := line.visit(nextAcc, c); !cont || err != nil {
					return res, cont, err
				}
			}
		}
		return acc, true, nil
	case yaml.ScalarNode:
		return acc, false, nil
	default:
		return "", false, fmt.Errorf("unhandled node %q", nodeKind(node))
	}
}
