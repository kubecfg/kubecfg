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

// Types and constants pertaining to the various ways we can pass variables to a jsonnet VM engine.
package vars

import (
	"fmt"

	"github.com/google/go-jsonnet"
)

type Type int

const (
	// --ext-*
	Ext Type = iota
	// --tla-*
	TLA
)

type ExpressionType int

const (
	// --*-str
	String ExpressionType = iota
	// --*-code
	Code
)

type Source int

const (
	// --*-*
	Literal Source = iota
	// --*-*-file
	File
)

type Var struct {
	Typ    Type
	Expr   ExpressionType
	Source Source
	Name   string
	Value  string
}

func New(typ Type, expr ExpressionType, source Source, name, value string) Var {
	return Var{typ, expr, source, name, value}
}

func (v *Var) Setter() func(*jsonnet.VM, string, string) {
	// when the source type is file, the caller will turn the value into an "import" expression
	// thus we need to call the "*Code" flavour.
	mapping := map[Var]func(*jsonnet.VM, string, string){
		{Ext, String, Literal, "", ""}: (*jsonnet.VM).ExtVar,
		{Ext, String, File, "", ""}:    (*jsonnet.VM).ExtCode,
		{Ext, Code, Literal, "", ""}:   (*jsonnet.VM).ExtCode,
		{Ext, Code, File, "", ""}:      (*jsonnet.VM).ExtCode,

		{TLA, String, Literal, "", ""}: (*jsonnet.VM).TLAVar,
		{TLA, String, File, "", ""}:    (*jsonnet.VM).TLACode,
		{TLA, Code, Literal, "", ""}:   (*jsonnet.VM).TLACode,
		{TLA, Code, File, "", ""}:      (*jsonnet.VM).TLACode,
	}
	s, found := mapping[Var{v.Typ, v.Expr, v.Source, "", ""}]
	if !found {
		panic(fmt.Sprintf("internal error: didn't update Setter to match enum types: %#v", v))
	}
	return s
}
