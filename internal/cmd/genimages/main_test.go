package main

import (
	"bytes"
	"os"
	"testing"
)

func TestGeneratedConstantsMatchData(t *testing.T) {
	input, err := os.ReadFile("../../../runtime-images.json")
	if err != nil {
		t.Fatal(err)
	}
	output, err := generate(input)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := os.ReadFile("../../../images_generated.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output, existing) {
		t.Fatal("run go generate . to synchronize pinned constants")
	}
	for _, bad := range [][]byte{[]byte(`null`), []byte(`{}`), []byte(`{"DefaultEmulatorImage":"latest"}`), append(input, []byte(`{}`)...)} {
		if _, err := generate(bad); err == nil {
			t.Fatalf("accepted invalid input: %s", bad)
		}
	}
}
