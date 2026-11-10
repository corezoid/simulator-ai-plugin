package sim

// The expression language is also used outside the simulation, by engines
// that let a user state a condition as data. These are its entry points.

// CheckExpr reports whether src is a well-formed expression.
func CheckExpr(src string) error {
	_, err := compileExpr(src)
	return err
}

// EvalExpr evaluates an expression over the given names and functions. Numbers
// are *big.Rat, texts string, records *OMap, lists []any.
func EvalExpr(src string, names map[string]any, funcs map[string]Func) (any, error) {
	return evaluate(src, names, funcs)
}

// Truthy reports whether a value counts as true in a condition.
func Truthy(v any) bool { return truthy(v) }
