package term

import (
	"os"
	"testing"
)

func TestSizeNotTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, _, ok := Size(f.Fd()); ok {
		t.Fatal("regular file reported as terminal")
	}
}

func TestPixelSizeNotTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, _, ok := PixelSize(f.Fd()); ok {
		t.Fatal("regular file reported as terminal")
	}
}
