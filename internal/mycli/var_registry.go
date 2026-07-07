package mycli

import (
	"fmt"
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

// registerAll builds the live registry from the declarative varDefs table
// followed by any feature-contributed varDefs (issue #778). Names and aliases
// must be unique case-folded across the whole set; a collision is a programming
// error (a core/core or core/feature name clash that would otherwise silently
// overwrite a handler) and panics rather than corrupting the registry.
func (r *VarRegistry) registerAll() {
	sv := r.sv
	register := func(def *varDef) {
		rv := &registeredVar{
			def: def,
			v:   def.bind(sv),
		}
		if def.bindAdd != nil {
			rv.add = def.bindAdd(sv)
		}
		r.putUnique(def.name, def.name, rv)
		// Aliases are extra keys pointing at the same registeredVar so lookups
		// (Get/Set/Add) resolve them, but listings iterate varDefs by canonical
		// name and therefore never surface aliases.
		for _, alias := range def.aliases {
			r.putUnique(alias, def.name, rv)
		}
	}
	for i := range varDefs {
		register(&varDefs[i])
	}
	for i := range sv.featureVarDefs {
		register(&sv.featureVarDefs[i])
	}
}

// putUnique inserts rv under the case-folded key, panicking if the key is
// already registered. sourceName is the canonical variable name being
// registered, for a clearer diagnostic when name != key (an alias collision).
func (r *VarRegistry) putUnique(key, sourceName string, rv *registeredVar) {
	upper := strings.ToUpper(key)
	if _, exists := r.vars[upper]; exists {
		panic(fmt.Sprintf("var registry: duplicate variable name %q (case-folded) while registering %q", upper, sourceName))
	}
	r.vars[upper] = rv
}

// lookupDef returns the declarative metadata for name (case-insensitive), or
// nil if the variable is unknown. Used by callers that need to consult policy
// (e.g. SET LOCAL eligibility) before touching the handler.
func (r *VarRegistry) lookupDef(name string) *varDef {
	rv, ok := r.vars[strings.ToUpper(name)]
	if !ok {
		return nil
	}
	return rv.def
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

	if err := r.checkSetPolicy(rv.def); err != nil {
		return err
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

// checkSetPolicy enforces the def's mutation policy (read-only, session-init-only,
// transaction-guarded) that governs whether a value may be changed right now. It
// is the single source of truth shared by both Set (`SET X = ...`) and Add
// (`SET X += ...`) so the two entry points cannot diverge: an ADD must never
// bypass a guard that a plain SET enforces. r.sv.inTransaction is nil until a
// session is created, so it doubles as the "session exists" signal for initOnly.
func (r *VarRegistry) checkSetPolicy(def *varDef) error {
	switch {
	case !def.settable():
		return errSetterReadOnly
	case def.initOnly && r.sv.inTransaction != nil:
		return &errSetterInitOnly{Name: def.name}
	case def.txnGuard && r.sv.inTransaction != nil && r.sv.inTransaction():
		return errSetterInTransaction
	}
	return nil
}

// Add performs ADD operation on a variable
func (r *VarRegistry) Add(name, value string) error {
	upperName := strings.ToUpper(name)

	// First check if the variable exists
	rv, ok := r.vars[upperName]
	if !ok {
		return &ErrUnknownVariable{Name: name}
	}

	// ADD is a mutation, so it must clear the same policy guards as SET; otherwise
	// `SET X += ...` would be a latent bypass of read-only/init-only/txn-guard.
	// There is no `SET LOCAL X += ...` grammar (the SET LOCAL pattern only accepts
	// `=`), so localAllowed() has no ADD path to enforce here.
	if err := r.checkSetPolicy(rv.def); err != nil {
		return err
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

// ListVariables returns a map of all variables with their current values.
// It iterates varDefs by canonical name so aliases are excluded, and skips
// variables whose Get reports the value as unavailable (e.g. multi-valued
// COMMIT_RESPONSE, whose columns are merged in separately via ListMultiValues).
func (r *VarRegistry) ListVariables() map[string]string {
	result := make(map[string]string)
	for i := range varDefs {
		name := strings.ToUpper(varDefs[i].name)
		value, err := r.vars[name].v.Get()
		if err == nil {
			result[name] = value
		}
	}
	return result
}

// ListMultiValues returns the merged GetMulti() output of every registered
// MultiValueVar whose value is currently available. Keys may intentionally
// collide with single-valued variables (COMMIT_RESPONSE's COMMIT_TIMESTAMP
// overrides the plain COMMIT_TIMESTAMP row in SHOW VARIABLES).
func (r *VarRegistry) ListMultiValues() map[string]string {
	result := make(map[string]string)
	for i := range varDefs {
		mv, ok := r.vars[strings.ToUpper(varDefs[i].name)].v.(MultiValueVar)
		if !ok {
			continue
		}
		values, err := mv.GetMulti()
		if err != nil {
			continue
		}
		maps.Copy(result, values)
	}
	return result
}

// ListVariableInfo returns information about all variables
func (r *VarRegistry) ListVariableInfo() map[string]struct {
	Description   string
	ReadOnly      bool
	CanAdd        bool
	Unimplemented bool
} {
	result := make(map[string]struct {
		Description   string
		ReadOnly      bool
		CanAdd        bool
		Unimplemented bool
	})

	// Iterate by canonical name so aliases are excluded from the listing. Both
	// the core varDefs table and any feature-contributed varDefs (issue #778) are
	// listed, so generated docs and HELP VARIABLES cover the full registered set.
	addRow := func(def *varDef) {
		name := strings.ToUpper(def.name)
		rv := r.vars[name]
		// Unimplemented status is derived from the bound handler type, not from a
		// hardcoded name list, so generated docs stay honest as vars come and go.
		_, unimplemented := rv.v.(*UnimplementedVar)
		result[name] = struct {
			Description   string
			ReadOnly      bool
			CanAdd        bool
			Unimplemented bool
		}{
			Description:   rv.def.desc,
			ReadOnly:      !rv.def.settable(),
			CanAdd:        rv.add != nil,
			Unimplemented: unimplemented,
		}
	}
	for i := range varDefs {
		addRow(&varDefs[i])
	}
	for i := range r.sv.featureVarDefs {
		addRow(&r.sv.featureVarDefs[i])
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
