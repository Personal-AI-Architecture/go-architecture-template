package conformance_test

import (
	"testing"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/checks"
)

func TestImportAndProviderBoundaries(t *testing.T) {
	if errors := checks.CheckImports("../.."); len(errors) > 0 {
		t.Fatalf("import check failed: %v", errors)
	}
	if errors := checks.CheckLockIn("../.."); len(errors) > 0 {
		t.Fatalf("lock-in check failed: %v", errors)
	}
}
