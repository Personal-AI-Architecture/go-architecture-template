package conformance_test

import (
	"testing"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/checks"
)

func TestPinnedPAAContractsAndSchemas(t *testing.T) {
	if errors := checks.CheckContracts("../..", ""); len(errors) > 0 {
		t.Fatalf("contract check failed: %v", errors)
	}
}
