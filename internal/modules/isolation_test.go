package modules

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every module keeps organizations apart (CLAUDE.md: every query filters by
// organization_id), and proves it with a test that tries a second
// organization. This fails when a module's tests never do (build guide 6.3).
var isolationPhrase = regexp.MustCompile(`(?i)(another|other|second) org(ani[sz]ation)?('s)?\b|isolat`)

// Packages under internal/modules that aren't tenant modules.
var notTenantModules = map[string]string{
	"kit":      "shared helpers; their own tests cover organization scoping where they touch data",
	"operator": "the cross-organization console for Weline staff; its tests prove only operators get in",
}

func TestEveryModuleTestsASecondOrganization(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, e := range entries {
		if !e.IsDir() || notTenantModules[e.Name()] != "" {
			continue
		}
		tests, err := filepath.Glob(filepath.Join(e.Name(), "*_test.go"))
		require.NoError(t, err)
		found := false
		for _, f := range tests {
			b, err := os.ReadFile(f)
			require.NoError(t, err)
			if isolationPhrase.Match(b) {
				found = true
				break
			}
		}
		assert.True(t, found, "module %s has no test that tries another organization (name it so: \"another organization …\")", e.Name())
	}
}
