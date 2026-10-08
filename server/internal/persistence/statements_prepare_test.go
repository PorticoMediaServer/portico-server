package persistence

import (
	"database/sql"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A statement that names a column or table the schema does not have fails only
// when it runs, and a path that runs rarely can carry one for months. This
// prepares every complete SQL statement written as a literal in the server's
// production code (a string literal, or literals joined with +) against the
// baseline schema and fails on a missing table, column or function. Fragments
// assembled at run time fail to parse and are skipped; the packages' own tests
// cover those.
type productionStatement struct{ where, text string }

// productionStatements are the complete SQL statements written as literals in
// the server's production code, with the tables and functions that exist only
// at run time (temporary tables, functions another package registers).
func productionStatements(t *testing.T) ([]productionStatement, map[string]bool) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	start := regexp.MustCompile(`(?is)^\s*(SELECT|INSERT|UPDATE|DELETE|WITH|REPLACE)\b`)
	// Tables a statement creates at run time (temporary work tables) and
	// functions another package registers with the driver.
	created := regexp.MustCompile(`(?i)CREATE\s+(?:TEMP|TEMPORARY)\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(\w+)`)
	registered := regexp.MustCompile(`Register\w*Function\("(\w+)"`)
	var statements []productionStatement
	transient := map[string]bool{"table:sqlite_stat1": true}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasPrefix(d.Name(), "zz_local_") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range registered.FindAllStringSubmatch(string(source), -1) {
			transient["function:"+strings.ToLower(m[1])] = true
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, source, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(file, func(n ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			text, ok := literal(expr)
			if !ok {
				// A statement assembled at run time ("SELECT …" + where): its
				// literal parts are not statements of their own.
				if b, isAdd := expr.(*ast.BinaryExpr); isAdd && b.Op == token.ADD {
					return false
				}
				return true
			}
			for _, m := range created.FindAllStringSubmatch(text, -1) {
				transient["table:"+strings.ToLower(m[1])] = true
			}
			if start.MatchString(text) {
				statements = append(statements, productionStatement{rel + ":" + strconv.Itoa(fset.Position(expr.Pos()).Line), text})
			}
			// A folded expression's parts are not statements of their own.
			return false
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return statements, transient
}

func TestProductionStatementsPrepareAgainstTheBaseline(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "statements.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements, transient := productionStatements(t)
	// The title dataset client (internal/enrichment) also reads the dataset
	// file, whose schema is its published contract (testdata/schema.sql):
	// its statements prepare against one schema or the other.
	dataset, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dataset.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer dataset.Close()
	contract, err := os.ReadFile(filepath.Join("..", "enrichment", "testdata", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = dataset.Exec(string(contract)); err != nil {
		t.Fatal(err)
	}
	// A delta is applied from a second file of the same schema, attached.
	dataset.SetMaxOpenConns(1)
	deltaPath := filepath.Join(t.TempDir(), "delta.sqlite")
	delta, err := sql.Open("sqlite", deltaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = delta.Exec(string(contract)); err != nil {
		t.Fatal(err)
	}
	delta.Close()
	if _, err = dataset.Exec(`ATTACH DATABASE ? AS delta`, deltaPath); err != nil {
		t.Fatal(err)
	}
	missing := regexp.MustCompile(`no such (table|column|function)`)
	name := regexp.MustCompile(`no such (table|column|function): (?:\w+\.)?(\w+)`)
	checked := 0
	for _, s := range statements {
		stmt, err := db.Prepare(s.text)
		if err == nil {
			stmt.Close()
			checked++
			continue
		}
		if !missing.MatchString(err.Error()) {
			continue
		}
		if strings.Contains(filepath.ToSlash(s.where), "internal/enrichment/") {
			if other, e := dataset.Prepare(s.text); e == nil {
				other.Close()
				checked++
				continue
			}
		}
		if m := name.FindStringSubmatch(err.Error()); m != nil && m[1] != "column" && transient[m[1]+":"+strings.ToLower(m[2])] {
			continue
		}
		t.Errorf("%s: %v\n\t%s", s.where, err, strings.Join(strings.Fields(s.text), " "))
	}
	if checked < 1000 {
		t.Fatalf("only %d statements prepared; the scan is broken", checked)
	}
	t.Logf("%d statements prepared", checked)
}

// literal folds a string literal, or literals joined with +, into its text.
func literal(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		return s, err == nil
	case *ast.ParenExpr:
		return literal(v.X)
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		a, ok := literal(v.X)
		if !ok {
			return "", false
		}
		b, ok := literal(v.Y)
		return a + b, ok
	}
	return "", false
}
