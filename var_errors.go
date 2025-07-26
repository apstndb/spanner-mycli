package main

import "errors"

// Common errors for variable operations
var (
	errSetterReadOnly      = errors.New("variable is read-only")
	errSetterInTransaction = errors.New("can't change variable when there is an active transaction")
)
