package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestKDFAdmissionUsesOneCeilingWithMutationReservation(t *testing.T) {
	admission := newKDFAdmission(2, 8)
	releaseCompare, err := admission.acquire(t.Context(), kdfLaneCompare)
	if err != nil {
		t.Fatalf("acquire first compare: %v", err)
	}
	defer releaseCompare()

	queued := make(chan struct{})
	acquired := make(chan func(), 1)
	go func() {
		close(queued)
		release, acquireErr := admission.acquire(t.Context(), kdfLaneCompare)
		if acquireErr == nil {
			acquired <- release
		}
	}()
	<-queued
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		admission.mu.Lock()
		queuedCompares := admission.queuedLocked(kdfLaneCompare)
		admission.mu.Unlock()
		if queuedCompares == 1 {
			break
		}
		runtime.Gosched()
	}

	// The second physical slot is reserved inside the same capacity=2
	// ceiling, so authenticated mutation work is admitted despite the login
	// compare backlog.
	releaseMutation, err := admission.acquire(t.Context(), kdfLaneMutation)
	if err != nil {
		t.Fatalf("reserved mutation slot was unavailable: %v", err)
	}
	admission.mu.Lock()
	if admission.active != 2 || admission.activeCompare != 1 || admission.activeMutation != 1 {
		t.Fatalf("unexpected active accounting: total=%d compare=%d mutation=%d", admission.active, admission.activeCompare, admission.activeMutation)
	}
	admission.mu.Unlock()
	releaseMutation()
	releaseCompare()
	select {
	case release := <-acquired:
		release()
	case <-time.After(time.Second):
		t.Fatal("queued compare did not make progress")
	}
}

func TestKDFAdmissionCancelledImmediateDoesZeroWork(t *testing.T) {
	admission := newKDFAdmission(2, 4)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	work := 0
	release, err := admission.acquire(ctx, kdfLaneCompare)
	if !errors.Is(err, errKDFCancelled) || release != nil {
		t.Fatalf("cancelled immediate acquire release=%v err=%v", release != nil, err)
	}
	if work != 0 {
		t.Fatalf("cancelled immediate acquire ran work=%d", work)
	}
	admission.mu.Lock()
	defer admission.mu.Unlock()
	if admission.active != 0 || admission.activeCompare != 0 || admission.activeMutation != 0 || len(admission.waiters) != 0 {
		t.Fatalf("cancelled immediate acquire leaked accounting: active=%d compare=%d mutation=%d waiters=%d", admission.active, admission.activeCompare, admission.activeMutation, len(admission.waiters))
	}
}

func TestKDFAdmissionCancellationConcurrentWithGrantReleasesExactlyOnce(t *testing.T) {
	admission := newKDFAdmission(1, 4)
	releaseFirst, err := admission.acquire(t.Context(), kdfLaneCompare)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		release, acquireErr := admission.acquire(ctx, kdfLaneCompare)
		if release != nil {
			release()
		}
		result <- acquireErr
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		admission.mu.Lock()
		queued := admission.queuedLocked(kdfLaneCompare)
		admission.mu.Unlock()
		if queued == 1 {
			break
		}
		runtime.Gosched()
	}
	cancel()
	releaseFirst()
	if err := <-result; !errors.Is(err, errKDFCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled grant err=%v", err)
	}
	admission.mu.Lock()
	defer admission.mu.Unlock()
	if admission.active != 0 || admission.activeCompare != 0 || admission.activeMutation != 0 || len(admission.waiters) != 0 {
		t.Fatalf("cancelled grant accounting: active=%d compare=%d mutation=%d waiters=%d", admission.active, admission.activeCompare, admission.activeMutation, len(admission.waiters))
	}
}

func TestKDFAdmissionBoundsEachLaneAndHonorsCancellation(t *testing.T) {
	admission := newKDFAdmission(2, 4) // compare queue=3, mutation queue=1
	releaseCompare, _ := admission.acquire(t.Context(), kdfLaneCompare)
	releaseMutation, _ := admission.acquire(t.Context(), kdfLaneMutation)
	defer releaseCompare()
	defer releaseMutation()

	mutationQueued := make(chan error, 1)
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		_, err := admission.acquire(ctx, kdfLaneMutation)
		mutationQueued <- err
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		admission.mu.Lock()
		count := admission.queuedLocked(kdfLaneMutation)
		admission.mu.Unlock()
		if count == 1 {
			break
		}
		runtime.Gosched()
	}
	if _, err := admission.acquire(t.Context(), kdfLaneMutation); !errors.Is(err, errKDFUnavailable) {
		t.Fatalf("over-capacity mutation queue error=%v", err)
	}
	cancel()
	if err := <-mutationQueued; !errors.Is(err, errKDFCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter error=%v", err)
	}
}

func TestProductionBcryptInventoryIsConfinedToAdmittedImplementations(t *testing.T) {
	// Resolve imports by package path and use go/types object identity.  An AST
	// check for the spelling "bcrypt" misses aliases, dot imports, and a
	// package-level/function-local function value assigned from bcrypt.
	type packageJSON struct {
		Dir        string
		ImportPath string
		GoFiles    []string
		CgoFiles   []string
	}
	goMod := exec.Command("go", "env", "GOMOD")
	moduleOutput, err := goMod.Output()
	if err != nil {
		t.Fatalf("locate Go module: %v", err)
	}
	repoRoot := filepath.Dir(string(bytes.TrimSpace(moduleOutput)))
	list := exec.Command("go", "list", "-json", "./...")
	list.Dir = repoRoot
	output, err := list.Output()
	if err != nil {
		t.Fatalf("go list production packages: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []packageJSON
	for decoder.More() {
		var pkg packageJSON
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatalf("decode go list package: %v", err)
		}
		packages = append(packages, pkg)
	}
	expected := map[string]map[string]map[string]int{
		"internal/app/password_kdf.go": {
			"hashAccountPassword":           {"GenerateFromPassword": 1},
			"verifyAccountPasswordSnapshot": {"CompareHashAndPassword": 1},
		},
		"internal/app/profiles.go": {
			"hashLocalProfilePIN":       {"GenerateFromPassword": 1},
			"verifyLocalProfilePINHash": {"CompareHashAndPassword": 1},
		},
		"internal/app/kdf_upgrade.go": {
			"runPasswordHashUpgrades": {"GenerateFromPassword": 1},
		},
	}
	observed := map[string]map[string]map[string]int{}
	var violations []string
	for _, pkg := range packages {
		fset := token.NewFileSet()
		files := make([]*ast.File, 0, len(pkg.GoFiles)+len(pkg.CgoFiles))
		paths := append(append([]string{}, pkg.GoFiles...), pkg.CgoFiles...)
		for _, name := range paths {
			filename := filepath.Join(pkg.Dir, name)
			parsed, parseErr := parser.ParseFile(fset, filename, nil, 0)
			if parseErr != nil {
				t.Fatalf("parse %s: %v", filename, parseErr)
			}
			files = append(files, parsed)
		}
		if len(files) == 0 {
			continue
		}
		for _, file := range files {
			bcryptAlias := ""
			bcryptDotImport := false
			for _, imported := range file.Imports {
				if imported.Path.Value != `"golang.org/x/crypto/bcrypt"` {
					continue
				}
				bcryptAlias = "bcrypt"
				if imported.Name != nil {
					bcryptAlias = imported.Name.Name
					bcryptDotImport = bcryptAlias == "."
				}
			}
			if bcryptAlias == "" {
				continue
			}
			for _, declaration := range file.Decls {
				function := "<package>"
				if current, ok := declaration.(*ast.FuncDecl); ok {
					function = current.Name.Name
				}
				parents := map[ast.Node]ast.Node{}
				var stack []ast.Node
				ast.Inspect(declaration, func(node ast.Node) bool {
					if node == nil {
						stack = stack[:len(stack)-1]
						return true
					}
					if len(stack) != 0 {
						parents[node] = stack[len(stack)-1]
					}
					stack = append(stack, node)
					return true
				})
				ast.Inspect(declaration, func(node ast.Node) bool {
					method := ""
					directCall := false
					switch current := node.(type) {
					case *ast.SelectorExpr:
						qualifier, ok := current.X.(*ast.Ident)
						if !ok || bcryptDotImport || qualifier.Name != bcryptAlias {
							return true
						}
						method = current.Sel.Name
						call, ok := parents[current].(*ast.CallExpr)
						directCall = ok && call.Fun == current
					case *ast.Ident:
						if !bcryptDotImport {
							return true
						}
						method = current.Name
						call, ok := parents[current].(*ast.CallExpr)
						directCall = ok && call.Fun == current
					default:
						return true
					}
					if method != "CompareHashAndPassword" && method != "GenerateFromPassword" {
						return true
					}
					relative, _ := filepath.Rel(repoRoot, fset.Position(node.Pos()).Filename)
					if expected[relative][function][method] == 0 || !directCall {
						violations = append(violations, relative+":"+function+":"+method)
					} else {
						if observed[relative] == nil {
							observed[relative] = map[string]map[string]int{}
						}
						if observed[relative][function] == nil {
							observed[relative][function] = map[string]int{}
						}
						observed[relative][function][method]++
					}
					return true
				})
			}
		}
	}
	for filename, functions := range expected {
		for function, calls := range functions {
			for method, count := range calls {
				if got := observed[filename][function][method]; got != count {
					violations = append(violations, filename+":"+function+":"+method+":count")
				}
			}
		}
	}
	if len(violations) != 0 {
		t.Fatalf("bcrypt calls bypass process admission: %v", violations)
	}
}
