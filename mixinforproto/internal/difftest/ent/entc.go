//go:build ignore

// This file is the driverless differential harness's entc codegen entry
// point, run via go:generate in generate.go. Unlike the root module's
// entc.go files (e.g. internal/entconnecttest/update/ent/entc.go), this
// one declares NO entc extension: mixinforproto must never import the
// root module (that would invert the two-module dependency direction
// Phase 1 D-15/D-16/D-17 establishes), so this is plain, extension-free
// entc.Generate — the generated ent.Client here has no entconnect
// handler/interceptor layer, only the ent mutation pipeline hooks.go
// plugs into.
package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	if err := entc.Generate("./schema", &gen.Config{}); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
