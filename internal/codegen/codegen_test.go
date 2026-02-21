package codegen

import (
	"strings"
	"testing"
)

func TestGenerateCode(t *testing.T) {
	for _, n := range []int{1, 6, 12} {
		code, err := GenerateCode(n)
		if err != nil {
			t.Fatalf("GenerateCode(%d): %v", n, err)
		}
		if len(code) != n {
			t.Errorf("GenerateCode(%d): got len %d, want %d", n, len(code), n)
		}
		for _, c := range code {
			if !strings.ContainsRune(alphabet, c) {
				t.Errorf("GenerateCode: invalid char %q in %q", c, code)
			}
		}
	}
}
