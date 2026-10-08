package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"portico.local/server/internal/httpapi"
)

// dependenciesLeftToDefaults are the httpapi.Dependencies fields the served
// handler may leave unset, each because a nil value has a working default.
// Every other field must be wired: a nil service turns its routes into 503s
// (the G2 merge on 16 Sep dropped Downloads this way, and every server answered
// "Offline downloads are not configured" until 23 Sep).
var dependenciesLeftToDefaults = map[string]string{
	"PlaybackV1": "nil builds the Playback Protocol v1 service from the handler's own dependencies",
	"HTTPSRoute": "nil uses the built-in HTTPS route attestation",
}

func TestMainWiresEveryHTTPDependency(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The served handler's literal is the largest httpapi.Dependencies literal
	// in main.go; smaller ones build single-purpose helpers.
	var wired map[string]bool
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Dependencies" {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "httpapi" {
			return true
		}
		fields := map[string]bool{}
		for _, element := range lit.Elts {
			if kv, ok := element.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok {
					fields[key.Name] = true
				}
			}
		}
		if len(fields) > len(wired) {
			wired = fields
		}
		return true
	})
	if len(wired) == 0 {
		t.Fatal("main.go builds no httpapi.Dependencies literal")
	}
	kind := reflect.TypeOf(httpapi.Dependencies{})
	for i := 0; i < kind.NumField(); i++ {
		field := kind.Field(i)
		if !field.IsExported() || wired[field.Name] {
			continue
		}
		if _, defaulted := dependenciesLeftToDefaults[field.Name]; defaulted {
			continue
		}
		t.Errorf("main.go serves httpapi.Dependencies without %s (%s); wire it, or list it in dependenciesLeftToDefaults with the reason nil is safe", field.Name, field.Type)
	}
	for name := range dependenciesLeftToDefaults {
		if _, ok := kind.FieldByName(name); !ok {
			t.Errorf("dependenciesLeftToDefaults names %s, which httpapi.Dependencies no longer has", name)
		}
	}
}
