//go:build windows

package runner

import (
	"slices"
	"testing"
)

func TestFixBashPath_ConvertsWindowsSplitList(t *testing.T) {
	env := []string{`PATH=C:\Tools;D:\Bin`}
	got := fixBashPath(append([]string{}, env...), pathFlavorMSYS)

	// We only care about PATH value change; env length should remain 1.
	if len(got) != 1 {
		t.Fatalf("unexpected env len: %d", len(got))
	}

	want := "PATH=/c/Tools:/d/Bin"
	if got[0] != want {
		t.Fatalf("fixBashPath() = %q, want %q", got[0], want)
	}

	// sanity check: should still be one entry
	if !slices.Equal(got, []string{want}) {
		t.Fatalf("unexpected env: %v", got)
	}
}
