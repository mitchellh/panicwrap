# panicwrap

panicwrap is a Go library that re-executes a Go binary and watches the
stderr for a panic. When it finds a panic, it executes a user-defined
handler function. The library contains a handful of handlers in order to do
common tasks such as writing the panic to a file.
