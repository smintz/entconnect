// Package entc is the entconnect entc.Extension: it reads the committed
// FileDescriptorSet plus the ent schema graph (including
// mixinforproto's own SourceMessage/SourceField provenance annotations)
// and emits ConnectRPC handler implementations for the RPCs an ent
// schema binds via GetRPC/ListRPC/CreateRPC/UpdateRPC/DeleteRPC/Manual.
//
// Read this before you extend anything here:
//
//   - The extension itself NEVER imports a generated Connect package
//     (D-02). Every RPC binding crosses entc's schema-load JSON boundary
//     as a plain procedure string ("/pkg.Service/Method"); resolve.go is
//     the only place that string is turned into a protoreflect.MethodDescriptor,
//     and it does so purely against the committed FileDescriptorSet —
//     never via a Go import edge to the service's generated package.
//   - Emission happens through a gen.Hook writing files directly via
//     os.WriteFile (extension.go), not through Extension.Templates():
//     ent's Templates()/GraphTemplate mechanism cannot target a
//     subdirectory outside the app's own ent/ package (see extension.go's
//     own doc comment). Every hook-written file is run through
//     golang.org/x/tools/imports.Process before being written — ent's own
//     gofmt/goimports pass (assets.format()) never sees hook-written
//     files.
//   - Every generated file begins with the standard machine-recognisable
//     "// Code generated ... DO NOT EDIT." header — CRUD-06 requires
//     these files to be visibly not hand-editable, not merely documented
//     as such.
package entc
