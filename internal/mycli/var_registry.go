package mycli

import (
	"log/slog"
	"maps"
	"strconv"
	"strings"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/ast"
)

// registeredVar pairs a variable's declarative metadata (def) with its live
// handler (v) and optional ADD handler. Policy (read-only/scope) is enforced
// from def, so the handler only implements Get/Set.
type registeredVar struct {
	def *varDef
	v   Variable
	add func(string) error
}

// VarRegistry is a registry for system variables
type VarRegistry struct {
	vars map[string]*registeredVar
	sv   *systemVariables
}

// NewVarRegistry creates a new variable registry
func NewVarRegistry(sv *systemVariables) *VarRegistry {
	r := &VarRegistry{
		vars: make(map[string]*registeredVar),
		sv:   sv,
	}
	r.registerAll()
	return r
}

// registerAll builds the live registry from the declarative varDefs table.
func (r *VarRegistry) registerAll() {
	sv := r.sv
	for i := range varDefs {
		def := &varDefs[i]
		rv := &registeredVar{
			def: def,
			v:   def.bind(sv),
		}
		if def.bindAdd != nil {
			rv.add = def.bindAdd(sv)
		}
		r.vars[strings.ToUpper(def.name)] = rv
	}
}

// GetVariable retrieves the Variable handler by name, or nil if not found.
func (r *VarRegistry) GetVariable(name string) Variable {
	rv, ok := r.vars[strings.ToUpper(name)]
	if !ok {
		return nil
	}
	return rv.v
}

// Get retrieves a variable value
func (r *VarRegistry) Get(name string) (string, error) {
	rv, ok := r.vars[strings.ToUpper(name)]
	if !ok {
		return "", &ErrUnknownVariable{Name: name}
	}
	value, err := rv.v.Get()
	if err != nil {
		return "", err
	}
	return value, nil
}

// Set sets a variable value
func (r *VarRegistry) Set(name, value string, isGoogleSQL bool) error {
	upperName := strings.ToUpper(name)
	rv, ok := r.vars[upperName]
	if !ok {
		slog.Debug("Variable not found in registry", "name", upperName, "availableVars", maps.Keys(r.vars))
		return &ErrUnknownVariable{Name: name}
	}

	// Read-only enforcement lives here, driven by the def's scope/readOnly
	// metadata, rather than inside each handler's Set.
	if !rv.def.settable() {
		return errSetterReadOnly
	}

	// Parse GoogleSQL value if needed
	originalValue := value
	if isGoogleSQL {
		value = parseGoogleSQLValue(value)
	}

	slog.Debug("Registry.Set", "name", upperName, "originalValue", originalValue, "parsedValue", value, "isGoogleSQL", isGoogleSQL)

	err := rv.v.Set(value)
	slog.Debug("Registry.Set result", "name", upperName, "err", err)
	return err
}

// Add performs ADD operation on a variable
func (r *VarRegistry) Add(name, value string) error {
	upperName := strings.ToUpper(name)

	// First check if the variable exists
	rv, ok := r.vars[upperName]
	if !ok {
		return &ErrUnknownVariable{Name: name}
	}

	// Then check if it supports ADD
	if rv.add == nil {
		return &ErrAddNotSupported{Name: name}
	}
	return rv.add(value)
}

// GetDescription returns variable description
func (r *VarRegistry) GetDescription(name string) (string, error) {
	rv, ok := r.vars[strings.ToUpper(name)]
	if !ok {
		return "", &ErrUnknownVariable{Name: name}
	}
	return rv.def.desc, nil
}

// IsReadOnly checks if a variable is read-only
func (r *VarRegistry) IsReadOnly(name string) (bool, error) {
	rv, ok := r.vars[strings.ToUpper(name)]
	if !ok {
		return false, &ErrUnknownVariable{Name: name}
	}
	return !rv.def.settable(), nil
}

// ListVariables returns a map of all variables with their current values
func (r *VarRegistry) ListVariables() map[string]string {
	result := make(map[string]string)
	for name, rv := range r.vars {
		value, err := rv.v.Get()
		if err == nil {
			result[name] = value
		}
	}
	return result
}

// ListVariableInfo returns information about all variables
func (r *VarRegistry) ListVariableInfo() map[string]struct {
	Description string
	ReadOnly    bool
	CanAdd      bool
} {
	result := make(map[string]struct {
		Description string
		ReadOnly    bool
		CanAdd      bool
	})

	for name, rv := range r.vars {
		result[name] = struct {
			Description string
			ReadOnly    bool
			CanAdd      bool
		}{
			Description: rv.def.desc,
			ReadOnly:    !rv.def.settable(),
			CanAdd:      rv.add != nil,
		}
	}

	return result
}

// parseGoogleSQLValue parses GoogleSQL-style values using memefish
func parseGoogleSQLValue(value string) (result string) {
	value = strings.TrimSpace(value)

	// Protect against panics from memefish.
	// While memefish.ParseExpr normally returns errors, it panics in some cases:
	// - Unclosed string literals (e.g., 'hello or "world)
	// - Unclosed triple-quoted strings (e.g., ''')
	// Without this recovery, entering an unclosed string in SET statements
	// would crash spanner-mycli entirely.
	defer func() {
		if r := recover(); r != nil {
			// If memefish panics, return the original value
			result = value
		}
	}()

	// Try to parse as an expression using memefish
	expr, err := memefish.ParseExpr("", value)
	if err != nil {
		// If parsing fails, return the original value
		return value
	}

	// Handle different literal types
	switch lit := expr.(type) {
	case *ast.StringLiteral:
		return lit.Value
	case *ast.BoolLiteral:
		return strconv.FormatBool(lit.Value)
	default:
		// For other expressions, return the original value
		return value
	}
}
