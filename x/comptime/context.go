package comptime

// Context provides explicit access to host operations during //rtg:comptime evaluation.
// Comptime functions/methods must take this as their first explicit parameter.
type Context interface {
	ReadFile(path string) (string, bool)
}
