//go:build ignore

// This file is the write-slice fixture's entc codegen entry point, run
// via go:generate in generate.go. Mirrors
// internal/entconnecttest/read/ent/entc.go exactly — see that file's own
// comment on the entc/entconnect package-name aliasing.
package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"

	entconnect "github.com/smintz/entconnect/entc"
)

func main() {
	ext, err := entconnect.NewExtension(
		entconnect.WithDescriptorSet("../../../../proto/descriptorset.binpb"),
	)
	if err != nil {
		log.Fatalf("entconnect: creating extension: %v", err)
	}
	if err := entc.Generate("./schema", &gen.Config{}, entc.Extensions(ext)); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
