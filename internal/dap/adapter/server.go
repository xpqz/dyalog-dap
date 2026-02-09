package adapter

// Server is the DAP adapter entry point.
type Server struct{}

// NewServer creates a DAP server instance.
func NewServer() *Server {
	return &Server{}
}
