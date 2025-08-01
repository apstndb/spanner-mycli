package sysvar

// GetValue returns a getter function that retrieves the value from the given pointer.
// This is useful for simplifying getter functions in system variable parsers.
//
// Example:
//   getter: GetValue(&sv.RPCPriority)
// Instead of:
//   getter: func() sppb.RequestOptions_Priority { return sv.RPCPriority }
func GetValue[T any](ptr *T) func() T {
	return func() T {
		return *ptr
	}
}

// SetValue returns a setter function that updates the value at the given pointer.
// This is useful for simplifying setter functions in system variable parsers.
//
// Example:
//   setter: SetValue(&sv.RPCPriority)
// Instead of:
//   setter: func(v sppb.RequestOptions_Priority) error { sv.RPCPriority = v; return nil }
func SetValue[T any](ptr *T) func(T) error {
	return func(v T) error {
		*ptr = v
		return nil
	}
}

// GetPointer returns a getter function that retrieves a pointer from a pointer-to-pointer.
// This is useful for fields that are already pointers (nullable values).
//
// Example:
//   getter: GetPointer(&sv.StatementTimeout)
// Instead of:
//   getter: func() *time.Duration { return sv.StatementTimeout }
func GetPointer[T any](ptr **T) func() *T {
	return func() *T {
		return *ptr
	}
}

// SetPointer returns a setter function that updates a pointer value.
// This is useful for fields that are pointers (nullable values).
//
// Example:
//   setter: SetPointer(&sv.StatementTimeout)
// Instead of:
//   setter: func(v *time.Duration) error { sv.StatementTimeout = v; return nil }
func SetPointer[T any](ptr **T) func(*T) error {
	return func(v *T) error {
		*ptr = v
		return nil
	}
}