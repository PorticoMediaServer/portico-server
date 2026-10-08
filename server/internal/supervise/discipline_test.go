package supervise

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A goroutine that can take the process down cannot be intercepted at runtime,
// so the rule is enforced where it is written, exactly as the write-gate rule is
// in internal/dbwork. There were ninety-nine bare `go` statements in this tree
// and every one of them was a whole-process kill waiting for the wrong input; a
// hundredth added next week would be too, and nothing would notice until a
// customer's server restarted for no reason anyone could name.
//
// Test files are exempt: a test is allowed to model a caller that does not
// contain its panics, which is exactly what the containment has to survive.
var bareGo = regexp.MustCompile(`^\s*go\s+(func\b|[A-Za-z_][A-Za-z0-9_.]*\()`)

func TestEveryGoroutineIsStartedThroughThisPackage(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	for _, tree := range []string{"internal", "cmd"} {
		walkErr := filepath.Walk(filepath.Join(root, tree), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			relative = filepath.ToSlash(relative)
			// This package is where containment is implemented; it starts the one
			// goroutine everything else goes through.
			if strings.HasPrefix(relative, "internal/supervise/") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for index, line := range strings.Split(string(data), "\n") {
				if bareGo.MatchString(line) {
					offenders = append(offenders, relative+":"+strconv.Itoa(index+1)+": "+strings.TrimSpace(line))
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatal(walkErr)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf(`a panic in any of these goroutines takes the whole process down.

Start them through supervise.Go (recover, log, count, end that goroutine) or
supervise.Supervise (the same, then restart with backoff — only for a loop whose
state is in the database rather than in the goroutine):

%s`, strings.Join(offenders, "\n"))
	}
}
