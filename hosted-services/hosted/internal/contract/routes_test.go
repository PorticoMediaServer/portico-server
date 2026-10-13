package contract_test

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"portico.local/apikit"
	"portico.local/hosted/internal/contract"
)

var patternForm = regexp.MustCompile(`^(GET|HEAD|POST|PUT|PATCH|DELETE) (/\S*)$`)
var claimPath = regexp.MustCompile(`^/v1/server-claims/[a-z-]+$`)

// servedRoutes reads internal/httpapi's source for every route it registers: the
// "METHOD /path" patterns given to mux.HandleFunc (including the
// `for _, method := range []string{...}` + method+" /path" form), and the paths
// ClaimHandler dispatches under the /v1/server-claims/ subtree, which accepts POST
// only. A registration the scanner does not understand fails the test instead of
// going unnoticed.
func servedRoutes(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("../httpapi/*.go")
	if err != nil {
		t.Fatal(err)
	}
	routes := map[string]bool{}
	claimSubtree := false
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		// A file a default build leaves out (devtrust-only development routes)
		// is not part of the served contract.
		if ok, err := build.Default.MatchFile(filepath.Dir(name), filepath.Base(name)); err != nil || !ok {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		loops := map[string][]string{}
		var stack []ast.Node
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			if loop, ok := n.(*ast.RangeStmt); ok {
				if value, ok := loop.Value.(*ast.Ident); ok {
					if list, ok := loop.X.(*ast.CompositeLit); ok {
						for _, element := range list.Elts {
							if s, ok := stringLiteral(element); ok {
								loops[value.Name] = append(loops[value.Name], s)
							}
						}
					}
				}
			}
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && (filepath.Base(name) == "claims.go" || filepath.Base(name) == "claim_web.go") {
				if s, _ := strconv.Unquote(lit.Value); claimPath.MatchString(s) {
					routes["POST "+s] = true
				}
			}
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "HandleFunc" && selector.Sel.Name != "Handle") {
				return true
			}
			var patterns []string
			switch arg := call.Args[0].(type) {
			case *ast.BasicLit:
				s, _ := stringLiteral(arg)
				patterns = []string{s}
			case *ast.BinaryExpr:
				method, okMethod := arg.X.(*ast.Ident)
				suffix, okSuffix := stringLiteral(arg.Y)
				if arg.Op != token.ADD || !okMethod || !okSuffix || len(loops[method.Name]) == 0 {
					t.Fatalf("%s: unrecognized route registration %s", fset.Position(call.Pos()), render(fset, arg))
				}
				for _, m := range loops[method.Name] {
					patterns = append(patterns, m+suffix)
				}
			default:
				t.Fatalf("%s: unrecognized route registration %s", fset.Position(call.Pos()), render(fset, arg))
			}
			for _, p := range patterns {
				if p == "/v1/server-claims/" {
					claimSubtree = true
					continue
				}
				m := patternForm.FindStringSubmatch(p)
				if m == nil {
					t.Fatalf("%s: unrecognized route pattern %q", fset.Position(call.Pos()), p)
				}
				if strings.HasPrefix(m[2], "/v1/") {
					routes[p] = true
				}
			}
			return true
		})
	}
	if !claimSubtree {
		t.Fatal("the /v1/server-claims/ subtree is no longer registered; update servedRoutes")
	}
	return routes
}

func stringLiteral(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

func render(fset *token.FileSet, e ast.Expr) string {
	return fmt.Sprintf("(%T at %s)", e, fset.Position(e.Pos()))
}

func TestContractDeclaresExactlyTheServedRoutes(t *testing.T) {
	served := servedRoutes(t)
	declared := map[string]bool{}
	for _, d := range contract.Definitions() {
		declared[d.Method+" "+d.Path] = true
	}
	var missing, stale []string
	for route := range served {
		if !declared[route] {
			missing = append(missing, route)
		}
	}
	for route := range declared {
		if !served[route] {
			stale = append(stale, route)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("internal/contract does not match internal/httpapi.\nServed but not declared (add to contract.Definitions):\n  %s\nDeclared but not served (remove from contract.Definitions):\n  %s",
			strings.Join(missing, "\n  "), strings.Join(stale, "\n  "))
	}
	// 95 since server membership, invitations and profiles left Hosted
	// (Spec — Hosted at Scale); a scanner that loses registrations finds far fewer.
	if len(served) < 90 {
		t.Fatalf("only %d routes found; the scanner is missing registrations", len(served))
	}
	t.Logf("%d routes served and declared", len(served))
}

var snakeCase = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

func TestDefinitionsAreWellFormed(t *testing.T) {
	ids := map[string]bool{}
	access := map[apikit.Access]bool{contract.Public: true, contract.AccountSession: true, contract.ServerCredential: true, contract.SignedRequest: true, contract.DNSFeed: true, contract.ProviderNotification: true}
	for _, d := range contract.Definitions() {
		switch {
		case !snakeCase.MatchString(d.ID):
			t.Errorf("%s: ID is not snake_case", d.ID)
		case ids[d.ID]:
			t.Errorf("%s: duplicate ID", d.ID)
		case d.Summary == "" || !strings.HasSuffix(d.Summary, "."):
			t.Errorf("%s: summary must be one sentence", d.ID)
		case !apikit.ValidScope(d.Requires):
			t.Errorf("%s: credential scope is missing or invalid", d.ID)
		case d.Access != contract.Public && d.Requires == apikit.ScopeOpen:
			t.Errorf("%s: protected authority cannot declare an open scope", d.ID)
		case !access[d.Access]:
			t.Errorf("%s: unknown access %q", d.ID, d.Access)
		case d.Status < 200 || d.Status > 299:
			t.Errorf("%s: status %d is not a success", d.ID, d.Status)
		case d.Status == 204 && d.Response.Name() != "NoBody":
			t.Errorf("%s: a 204 route must declare NoBody", d.ID)
		case (d.BodyLimit == 0) != (d.Request.Name() == "NoBody"):
			t.Errorf("%s: body limit %d does not match request %s", d.ID, d.BodyLimit, d.Request)
		case d.Method == "GET" && d.BodyLimit != 0:
			t.Errorf("%s: GET with a body", d.ID)
		}
		ids[d.ID] = true
		for _, typ := range []string{d.Request.String(), d.Response.String()} {
			if strings.Contains(typ, "struct {") || strings.HasPrefix(typ, "map[") {
				t.Errorf("%s: %s is not a named type", d.ID, typ)
			}
		}
		if _, err := apikit.Schema(d.Request); err != nil {
			t.Errorf("%s request: %v", d.ID, err)
		}
		if _, err := apikit.Schema(d.Response); err != nil {
			t.Errorf("%s response: %v", d.ID, err)
		}
	}
}
