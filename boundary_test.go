package agentskill

import (
	"go/build"
	"strings"
	"testing"
)

const (
	modulePath   = "github.com/ChristopherDavenport/agentskill"
	agenttoolPkg = "github.com/ChristopherDavenport/agenttool"
	openresp     = "github.com/ChristopherDavenport/openresponses"
	yamlPkg      = "go.yaml.in/yaml/v3"
	agentturnPkg = "github.com/ChristopherDavenport/agentturn"
)

// TestImportBoundary enforces the module's dependency rules: the root
// package imports openresponses, agenttool, the YAML parser and the
// standard library only; agentturn appears in test files alone; and
// the instructions package imports the standard library alone.
func TestImportBoundary(t *testing.T) {
	root, err := build.Default.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range root.Imports {
		switch {
		case isStandard(imp):
		case imp == agenttoolPkg, imp == openresp, imp == yamlPkg:
		default:
			t.Errorf("agentskill imports %q; only agenttool, openresponses, yaml and the standard library are allowed", imp)
		}
	}
	for _, imp := range append(root.TestImports, root.XTestImports...) {
		switch {
		case isStandard(imp):
		case strings.HasPrefix(imp, modulePath):
		case strings.HasPrefix(imp, agenttoolPkg), strings.HasPrefix(imp, openresp), imp == yamlPkg:
		case imp == agentturnPkg:
			// The tool's integration test drives the loop.
		default:
			t.Errorf("agentskill tests import %q; that is not an allowed test dependency", imp)
		}
	}

	instr, err := build.Default.ImportDir("instructions", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range instr.Imports {
		if !isStandard(imp) {
			t.Errorf("instructions imports %q; only the standard library is allowed", imp)
		}
	}
	for _, imp := range append(instr.TestImports, instr.XTestImports...) {
		if !isStandard(imp) && !strings.HasPrefix(imp, modulePath+"/instructions") {
			t.Errorf("instructions tests import %q; only the standard library is allowed", imp)
		}
	}
}

func isStandard(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return !strings.Contains(first, ".")
}
