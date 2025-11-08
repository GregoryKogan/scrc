package ports

import "scrc/internal/runtime"

// PreparedScript aliases the runtime.PreparedScript interface to preserve port compatibility.
type PreparedScript = runtime.PreparedScript

// Runner aliases the runtime.Engine interface.
type Runner = runtime.Engine
