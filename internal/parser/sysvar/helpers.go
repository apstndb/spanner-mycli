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

