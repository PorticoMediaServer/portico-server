package mediaexec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// processSpawns is every place in the server that starts a process other than
// through this package, with why it is not a media tool on untrusted input.
// SEC-11 / ARCH-MEDIA-09: FFmpeg and ffprobe start only through mediaexec. A
// new spawn anywhere fails this test until it is either routed through
// mediaexec or added here with a reason a reviewer accepts.
var processSpawns = map[string]string{
	// The decoder's own Linux sandbox, already inside mediaexec's policy.
	"internal/decoder/confinement_linux.go:ProbeConfinement":         "bubblewrap probe of the server's own helper; no media",
	"internal/decoder/confinement_linux.go:runOuter":                 "the trusted outer helper starting bubblewrap; the helper itself was started by mediaexec",
	"internal/decoder/confinement_linux.go:runInner":                 "the tool inside the bubblewrap sandbox the outer helper built",
	"internal/decoder/prepared_file.go:PreparedFileSandboxAvailable": "bubblewrap --help, to read its capabilities",
	// Helpers: the server binary re-executed for one bounded storage or subtitle
	// operation. They run media tools only through mediaexec argv.
	"internal/storage/client.go:ReadSmall":                                    "storage helper (server binary)",
	"internal/storage/client.go:run":                                          "storage helper (server binary)",
	"internal/storage/inventory_page.go:inventoryRequest":                     "storage helper (server binary)",
	"internal/storage/inventory_page_stream.go:produce":                       "storage helper (server binary)",
	"internal/storage/observed_playback.go:OpenObservedPlayback":              "storage helper (server binary)",
	"internal/storage/lyrics.go:ReadLyrics":                                   "storage helper (server binary); its ffprobe argv comes from mediaexec",
	"internal/storage/playback_stream.go:openPlayback":                        "storage helper (server binary)",
	"internal/storage/scan_content.go:scanHelperCommand":                      "storage helper (server binary); its tool argv comes from mediaexec",
	"internal/storage/playback_descriptor_unix.go:OpenPlaybackDescriptor":     "storage helper (server binary)",
	"internal/storage/playback_descriptor_unix.go:ValidatePlaybackDescriptor": "storage helper (server binary)",
	"internal/storage/versioned_playback.go:runVersion":                       "storage helper (server binary)",
	"internal/storage/root_registration_unix.go:RegisterRoot":                 "storage helper (server binary)",
	"internal/subtitles/acquisition.go:Import":                                "subtitle sidecar helper (server binary); reads text files, runs no tool",
	// Not media.
	"internal/mounts/service.go:reconcile":           "rclone mount guardian (server binary)",
	"internal/mounts/native.go:NativeHelper":         "rclone, inside the native helper, for a managed network mount",
	"internal/mounts/native.go:runOwned":             "rclone native helper (server binary)",
	"internal/mounts/native.go:approveExecutable":    "rclone version, to approve the configured executable",
	"internal/mounts/native.go:validateCandidate":    "rclone native helper (server binary)",
	"internal/mounts/guardian.go:Guardian":           "rclone under the guardian",
	"internal/networking/topology.go:defaultGateway": "/sbin/route on macOS",
	"internal/telemetry/command.go:runCommand":       "fixed read-only platform reporters",
	"internal/httpapi/fixture/fixture.go:cloneFile":  "test fixture copy (cp), not built into the server",
}

type spawn struct{ file, function string }

func TestEveryProcessSpawnIsMediaexecOrListed(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	var unlisted []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "testdata" || name == "node_modules" || strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/mediaexec/") {
			return nil
		}
		for _, s := range spawnsIn(t, path) {
			key := rel + ":" + s.function
			found[key] = true
			if _, ok := processSpawns[key]; !ok {
				unlisted = append(unlisted, key+" ("+s.file+")")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(unlisted)
	for _, key := range unlisted {
		t.Errorf("process started outside mediaexec: %s — route FFmpeg/ffprobe through mediaexec, or list a non-media spawn in processSpawns with its reason", key)
	}
	for key := range processSpawns {
		if !found[key] {
			t.Errorf("processSpawns lists %s, which no longer starts a process; remove it", key)
		}
	}
}

// spawnsIn finds exec.Command, exec.CommandContext, exec.Cmd literals,
// os.StartProcess and syscall/unix Exec, ForkExec and StartProcess, by the
// import path of the package they belong to.
func spawnsIn(t *testing.T, path string) []spawn {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	imports := map[string]string{}
	for _, spec := range file.Imports {
		p, _ := strconv.Unquote(spec.Path.Value)
		name := p[strings.LastIndex(p, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = p
	}
	starts := map[string][]string{
		"os/exec":               {"Command", "CommandContext"},
		"os":                    {"StartProcess"},
		"syscall":               {"Exec", "ForkExec", "StartProcess"},
		"golang.org/x/sys/unix": {"Exec"},
	}
	var out []spawn
	visit := func(owner string, node ast.Node) {
		ast.Inspect(node, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				// exec.Cmd{...} built by hand.
				if sel, ok := n.Type.(*ast.SelectorExpr); ok {
					if pkg, ok := sel.X.(*ast.Ident); ok && imports[pkg.Name] == "os/exec" && sel.Sel.Name == "Cmd" {
						out = append(out, spawn{file: "exec.Cmd{}", function: owner})
					}
				}
			case *ast.SelectorExpr:
				pkg, ok := n.X.(*ast.Ident)
				if !ok {
					return true
				}
				for _, name := range starts[imports[pkg.Name]] {
					if n.Sel.Name == name {
						out = append(out, spawn{file: pkg.Name + "." + name, function: owner})
					}
				}
			}
			return true
		})
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			visit(fn.Name.Name, fn)
		} else {
			visit("<package>", decl)
		}
	}
	return out
}
