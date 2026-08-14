package runtime

import "net/http"

// Route pairs a Connect procedure path with the http.Handler generated
// server wiring built for it (already wrapped in the fixed interceptor
// chain via Chain). This is what application code receives instead of
// an *ent.Client (CRUD-07/D-12).
type Route struct {
	Path    string
	Handler http.Handler
}

// Server holds the routes generated wiring registered. It exposes no
// accessor of any kind to the *ent.Client the generated wiring
// constructed — Routes()/Register() are its entire surface.
type Server struct {
	routes []Route
}

// NewServer builds a Server from the given routes. Generated wiring
// calls this once, after building each service's chain-wrapped handler
// via Chain + the generated <Service>connect.New<Service>Handler.
func NewServer(routes ...Route) *Server {
	return &Server{routes: routes}
}

// Routes returns the server's registered routes.
func (s *Server) Routes() []Route {
	return s.routes
}

// Register mounts every route on mux.
func (s *Server) Register(mux *http.ServeMux) {
	for _, r := range s.routes {
		mux.Handle(r.Path, r.Handler)
	}
}
