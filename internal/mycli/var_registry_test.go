package mycli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGoogleSQLValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// String literals
		{"single quoted string", "'hello'", "hello"},
		{"double quoted string", `"world"`, "world"},
		{"string with escapes", `'hello\nworld'`, "hello\nworld"},
		{"string with quotes", `"it's a test"`, "it's a test"},
		{"empty string", "''", ""},

		// Boolean literals
		{"TRUE uppercase", "TRUE", "true"},
		{"FALSE uppercase", "FALSE", "false"},
		{"true lowercase", "true", "true"},
		{"false lowercase", "false", "false"},
		{"True mixed case", "True", "true"},

		// Other values pass through
		{"number", "123", "123"},
		{"identifier", "SOME_VALUE", "SOME_VALUE"},
		{"NULL", "NULL", "NULL"},

		// Invalid expressions pass through
		{"unclosed quote", "'hello", "'hello"},
		{"mixed quotes", `"'hello'"`, "'hello'"},
		{"invalid syntax", "'''", "'''"},

		// Whitespace handling
		{"string with spaces", "  'hello'  ", "hello"},
		{"boolean with spaces", "  TRUE  ", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGoogleSQLValue(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRegistryAddEnforcesSetPolicy verifies that Add (`SET X += ...`) rejects the
// same policy-guarded variables that Set (`SET X = ...`) rejects, so ADD cannot
// become a bypass of read-only/init-only/txn-guard. Each case uses a small fake
// def with a bindAdd handler that must never run when the guard fires.
func TestRegistryAddEnforcesSetPolicy(t *testing.T) {
	t.Parallel()

	activeTxn := func() bool { return true }

	tests := []struct {
		name    string
		def     varDef
		txn     func() bool // systemVariables.inTransaction; nil means no session yet
		wantErr error
	}{
		{
			name:    "readOnly blocks add",
			def:     varDef{name: "FAKE_RO", scope: scopeSession, readOnly: true},
			txn:     nil,
			wantErr: errSetterReadOnly,
		},
		{
			name:    "initOnly blocks add after session creation",
			def:     varDef{name: "FAKE_INIT", scope: scopeSession, initOnly: true},
			txn:     activeTxn,
			wantErr: &errSetterInitOnly{Name: "FAKE_INIT"},
		},
		{
			name:    "txnGuard blocks add inside transaction",
			def:     varDef{name: "FAKE_TXN", scope: scopeSession, txnGuard: true},
			txn:     activeTxn,
			wantErr: errSetterInTransaction,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			addCalled := false
			def := tt.def
			r := &VarRegistry{
				sv: &systemVariables{inTransaction: tt.txn},
				vars: map[string]*registeredVar{
					strings.ToUpper(def.name): {
						def: &def,
						add: func(string) error {
							addCalled = true
							return nil
						},
					},
				},
			}

			err := r.Add(def.name, "1")
			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err)
			assert.False(t, addCalled, "add handler must not run when the policy guard rejects the ADD")
		})
	}
}

// TestRegistryAddAllowedWhenSettable confirms the guard does not over-block: a
// plain settable variable still runs its ADD handler.
func TestRegistryAddAllowedWhenSettable(t *testing.T) {
	t.Parallel()

	addCalled := false
	def := varDef{name: "FAKE_OK", scope: scopeSession}
	r := &VarRegistry{
		sv: &systemVariables{},
		vars: map[string]*registeredVar{
			strings.ToUpper(def.name): {
				def: &def,
				add: func(string) error {
					addCalled = true
					return nil
				},
			},
		},
	}

	require.NoError(t, r.Add(def.name, "1"))
	assert.True(t, addCalled, "add handler should run for a settable variable")
}
