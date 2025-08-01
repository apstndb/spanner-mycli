package sysvar

import "fmt"

// GetValue returns a getter function that retrieves the value from the given pointer.
// This is useful for simplifying getter functions in system variable parsers.
//
// Example:
//
//	getter: GetValue(&sv.RPCPriority)
//
// Instead of:
//
//	getter: func() sppb.RequestOptions_Priority { return sv.RPCPriority }
func GetValue[T any](ptr *T) func() T {
	return func() T {
		return *ptr
	}
}

// SetValue returns a setter function that updates the value at the given pointer.
// This is useful for simplifying setter functions in system variable parsers.
//
// Example:
//
//	setter: SetValue(&sv.RPCPriority)
//
// Instead of:
//
//	setter: func(v sppb.RequestOptions_Priority) error { sv.RPCPriority = v; return nil }
func SetValue[T any](ptr *T) func(T) error {
	return func(v T) error {
		*ptr = v
		return nil
	}
}

// SetSessionInitOnly returns a setter function that only allows changes before session creation.
// This is used for variables that configure client initialization behavior and cannot be
// changed after the session is established.
//
// Example:
//
//	setter: SetSessionInitOnly(&sv.EnableADCPlus, "CLI_ENABLE_ADC_PLUS", &sv.CurrentSession)
//
// Instead of:
//
//	setter: func(v bool) error {
//	  if sv.CurrentSession != nil {
//	    return fmt.Errorf("CLI_ENABLE_ADC_PLUS cannot be changed after session creation")
//	  }
//	  sv.EnableADCPlus = v
//	  return nil
//	}
func SetSessionInitOnly[T any, S any](ptr *T, varName string, sessionPtr **S) func(T) error {
	return func(v T) error {
		if *sessionPtr != nil {
			return fmt.Errorf("%s cannot be changed after session creation", varName)
		}
		*ptr = v
		return nil
	}
}
