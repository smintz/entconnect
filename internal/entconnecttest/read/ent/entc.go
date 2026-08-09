//go:build ignore

// This file is the walking-slice fixture's entc codegen entry point,
// run via go:generate in generate.go. It mirrors the shape
// entconnect.md/CONTEXT.md's own example uses:
//
//	entc.Generate("./schema", &gen.Config{...}, entc.Extensions(ext))
//
// with ext built from entconnect.NewExtension(entconnect.WithDescriptorSet(...)).
//
// Note the import aliasing: entgo.io/ent/entc and
// github.com/smintz/entconnect/entc both declare `package entc` — this
// file imports the former under its default name and the latter under
// the explicit alias "entconnect", exactly as the phase plan's own
// example does.
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
