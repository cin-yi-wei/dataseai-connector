package control

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTailLinesReturnsLastLines(t *testing.T) {
	path := writeLogFile(t, "one\ntwo\nthree\nfour\n")

	got, err := TailLines(path, 2)
	if err != nil {
		t.Fatalf("TailLines: %v", err)
	}
	want := []string{"three", "four"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TailLines = %#v, want %#v", got, want)
	}
}

func TestTailLinesNonPositiveCountReturnsNil(t *testing.T) {
	path := writeLogFile(t, "one\ntwo\n")

	got, err := TailLines(path, 0)
	if err != nil {
		t.Fatalf("TailLines: %v", err)
	}
	if got != nil {
		t.Fatalf("TailLines = %#v, want nil", got)
	}
}

func TestTailLinesHandlesFilesShorterThanCount(t *testing.T) {
	path := writeLogFile(t, "one\ntwo")

	got, err := TailLines(path, 10)
	if err != nil {
		t.Fatalf("TailLines: %v", err)
	}
	want := []string{"one", "two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TailLines = %#v, want %#v", got, want)
	}
}

func writeLogFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "connector.log")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write test log: %v", err)
	}
	return path
}
