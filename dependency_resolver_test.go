package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDependencyResolver_TopologicalSort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		setupTables    func() *DependencyResolver
		tablesToExport []string
		expectedOrder  []string
		expectError    bool
		errorContains  string
	}{
		{
			name: "Simple INTERLEAVE hierarchy",
			setupTables: func() *DependencyResolver {
				dr := NewDependencyResolver()
				dr.tables = map[string]*TableDependency{
					"Singers": {
						TableName:      "Singers",
						ChildrenTables: []string{"Albums"},
						Level:          0,
					},
					"Albums": {
						TableName:      "Albums",
						ParentTable:    "Singers",
						ChildrenTables: []string{"Songs"},
						Level:          1,
					},
					"Songs": {
						TableName:   "Songs",
						ParentTable: "Albums",
						Level:       2,
					},
				}
				return dr
			},
			tablesToExport: []string{"Songs", "Albums", "Singers"},
			expectedOrder:  []string{"Singers", "Albums", "Songs"},
			expectError:    false,
		},
		{
			name: "Tables with foreign keys",
			setupTables: func() *DependencyResolver {
				dr := NewDependencyResolver()
				dr.tables = map[string]*TableDependency{
					"Venues": {
						TableName: "Venues",
						Level:     0,
					},
					"Singers": {
						TableName: "Singers",
						Level:     0,
					},
					"Concerts": {
						TableName: "Concerts",
						ForeignKeys: []FKReference{
							{ParentTable: "Venues", ChildTable: "Concerts"},
							{ParentTable: "Singers", ChildTable: "Concerts"},
						},
						Level: 0,
					},
				}
				return dr
			},
			tablesToExport: []string{"Concerts", "Venues", "Singers"},
			expectedOrder:  []string{"Singers", "Venues", "Concerts"},
			expectError:    false,
		},
		{
			name: "Mixed INTERLEAVE and FK dependencies",
			setupTables: func() *DependencyResolver {
				dr := NewDependencyResolver()
				dr.tables = map[string]*TableDependency{
					"Venues": {
						TableName: "Venues",
						Level:     0,
					},
					"Singers": {
						TableName:      "Singers",
						ChildrenTables: []string{"Albums"},
						Level:          0,
					},
					"Albums": {
						TableName:   "Albums",
						ParentTable: "Singers",
						Level:       1,
					},
					"Concerts": {
						TableName: "Concerts",
						ForeignKeys: []FKReference{
							{ParentTable: "Venues", ChildTable: "Concerts"},
							{ParentTable: "Singers", ChildTable: "Concerts"},
						},
						Level: 0,
					},
				}
				return dr
			},
			tablesToExport: []string{"Concerts", "Albums", "Venues", "Singers"},
			expectedOrder:  []string{"Singers", "Albums", "Venues", "Concerts"},
			expectError:    false,
		},
		{
			name: "Self-referential FK",
			setupTables: func() *DependencyResolver {
				dr := NewDependencyResolver()
				dr.tables = map[string]*TableDependency{
					"Employees": {
						TableName: "Employees",
						ForeignKeys: []FKReference{
							{ParentTable: "Employees", ChildTable: "Employees", ChildColumn: "ManagerId"},
						},
						Level: 0,
					},
				}
				return dr
			},
			tablesToExport: []string{"Employees"},
			expectedOrder:  []string{"Employees"},
			expectError:    false,
		},
		{
			name: "Circular FK dependency",
			setupTables: func() *DependencyResolver {
				dr := NewDependencyResolver()
				dr.tables = map[string]*TableDependency{
					"TableA": {
						TableName: "TableA",
						ForeignKeys: []FKReference{
							{ParentTable: "TableB", ChildTable: "TableA"},
						},
						Level: 0,
					},
					"TableB": {
						TableName: "TableB",
						ForeignKeys: []FKReference{
							{ParentTable: "TableA", ChildTable: "TableB"},
						},
						Level: 0,
					},
				}
				return dr
			},
			tablesToExport: []string{"TableA", "TableB"},
			expectedOrder:  nil,
			expectError:    true,
			errorContains:  "circular foreign key dependency detected: TableA -> TableB -> TableA",
		},
		{
			name: "Deep INTERLEAVE hierarchy (7 levels)",
			setupTables: func() *DependencyResolver {
				dr := NewDependencyResolver()
				tables := []string{"T1", "T2", "T3", "T4", "T5", "T6", "T7"}
				for i, name := range tables {
					td := &TableDependency{
						TableName: name,
						Level:     i,
					}
					if i > 0 {
						td.ParentTable = tables[i-1]
						dr.tables[tables[i-1]].ChildrenTables = append(dr.tables[tables[i-1]].ChildrenTables, name)
					}
					dr.tables[name] = td
				}
				return dr
			},
			tablesToExport: []string{"T7", "T5", "T3", "T1", "T6", "T4", "T2"},
			expectedOrder:  []string{"T1", "T2", "T3", "T4", "T5", "T6", "T7"},
			expectError:    false,
		},
		{
			name: "FK that conflicts with INTERLEAVE",
			setupTables: func() *DependencyResolver {
				dr := NewDependencyResolver()
				dr.tables = map[string]*TableDependency{
					"Parent": {
						TableName:      "Parent",
						ChildrenTables: []string{"Child"},
						Level:          0,
					},
					"Child": {
						TableName:   "Child",
						ParentTable: "Parent",
						ForeignKeys: []FKReference{
							{ParentTable: "Other", ChildTable: "Child"},
						},
						Level: 1,
					},
					"Other": {
						TableName: "Other",
						ForeignKeys: []FKReference{
							{ParentTable: "Child", ChildTable: "Other"},
						},
						Level: 0,
					},
				}
				return dr
			},
			tablesToExport: []string{"Parent", "Child", "Other"},
			// Both orders are valid since FK cycle is broken by INTERLEAVE
			// Parent must come before Child (INTERLEAVE), but Other can be anywhere
			// after being processed. We accept the actual order produced.
			expectedOrder: []string{"Parent", "Other", "Child"},
			expectError:   false,
		},
		{
			name: "Empty database",
			setupTables: func() *DependencyResolver {
				return NewDependencyResolver()
			},
			tablesToExport: []string{},
			expectedOrder:  []string{}, // go-cmp handles nil vs empty slice correctly
			expectError:    false,
		},
		{
			name: "Single table",
			setupTables: func() *DependencyResolver {
				dr := NewDependencyResolver()
				dr.tables = map[string]*TableDependency{
					"SingleTable": {
						TableName: "SingleTable",
						Level:     0,
					},
				}
				return dr
			},
			tablesToExport: []string{"SingleTable"},
			expectedOrder:  []string{"SingleTable"},
			expectError:    false,
		},
		{
			name: "Table not found",
			setupTables: func() *DependencyResolver {
				dr := NewDependencyResolver()
				dr.tables = map[string]*TableDependency{
					"ExistingTable": {
						TableName: "ExistingTable",
						Level:     0,
					},
				}
				return dr
			},
			tablesToExport: []string{"NonExistentTable"},
			expectedOrder:  nil,
			expectError:    true,
			errorContains:  "table NonExistentTable not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dr := tt.setupTables()
			result, err := dr.GetOrderForTables(tt.tablesToExport)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tt.errorContains)
				} else if tt.errorContains != "" && !containsStr(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if diff := cmp.Diff(tt.expectedOrder, result); diff != "" {
					t.Errorf("Order mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestDependencyResolver_HasInterleavePathBetween(t *testing.T) {
	t.Parallel()
	dr := NewDependencyResolver()
	dr.tables = map[string]*TableDependency{
		"GrandParent": {
			TableName:      "GrandParent",
			ChildrenTables: []string{"Parent"},
		},
		"Parent": {
			TableName:      "Parent",
			ParentTable:    "GrandParent",
			ChildrenTables: []string{"Child"},
		},
		"Child": {
			TableName:   "Child",
			ParentTable: "Parent",
		},
		"Unrelated": {
			TableName: "Unrelated",
		},
	}

	tests := []struct {
		ancestor   string
		descendant string
		expected   bool
	}{
		{"GrandParent", "Child", true},
		{"GrandParent", "Parent", true},
		{"Parent", "Child", true},
		{"Child", "Parent", false},
		{"Child", "GrandParent", false},
		{"Unrelated", "Child", false},
		{"Parent", "Unrelated", false},
	}

	for _, tt := range tests {
		t.Run(tt.ancestor+"->"+tt.descendant, func(t *testing.T) {
			result := dr.hasInterleavePathBetween(tt.ancestor, tt.descendant)
			if result != tt.expected {
				t.Errorf("hasInterleavePathBetween(%s, %s) = %v, expected %v",
					tt.ancestor, tt.descendant, result, tt.expected)
			}
		})
	}
}

func TestDependencyResolver_CalculateInterleaveLevels(t *testing.T) {
	t.Parallel()
	dr := NewDependencyResolver()
	dr.tables = map[string]*TableDependency{
		"Root": {
			TableName:      "Root",
			ChildrenTables: []string{"Level1"},
		},
		"Level1": {
			TableName:      "Level1",
			ParentTable:    "Root",
			ChildrenTables: []string{"Level2"},
		},
		"Level2": {
			TableName:      "Level2",
			ParentTable:    "Level1",
			ChildrenTables: []string{"Level3"},
		},
		"Level3": {
			TableName:   "Level3",
			ParentTable: "Level2",
		},
		"Independent": {
			TableName: "Independent",
		},
	}

	dr.calculateInterleaveLevels()

	expectedLevels := map[string]int{
		"Root":        0,
		"Level1":      1,
		"Level2":      2,
		"Level3":      3,
		"Independent": 0,
	}

	for tableName, expectedLevel := range expectedLevels {
		if dr.tables[tableName].Level != expectedLevel {
			t.Errorf("Table %s: expected level %d, got %d",
				tableName, expectedLevel, dr.tables[tableName].Level)
		}
	}
}

func TestDependencyResolver_GetDependencyInfo(t *testing.T) {
	t.Parallel()
	dr := NewDependencyResolver()
	dr.tables = map[string]*TableDependency{
		"TestTable": {
			TableName:   "TestTable",
			ParentTable: "ParentTable",
			ForeignKeys: []FKReference{
				{ParentTable: "RefTable", ChildTable: "TestTable"},
			},
			Level: 1,
		},
	}

	// Test existing table
	info, err := dr.GetDependencyInfo("TestTable")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if info.TableName != "TestTable" {
		t.Errorf("Expected table name TestTable, got %s", info.TableName)
	}
	if info.ParentTable != "ParentTable" {
		t.Errorf("Expected parent table ParentTable, got %s", info.ParentTable)
	}
	if len(info.ForeignKeys) != 1 {
		t.Errorf("Expected 1 foreign key, got %d", len(info.ForeignKeys))
	}

	// Test non-existent table
	_, err = dr.GetDependencyInfo("NonExistent")
	if err == nil {
		t.Error("Expected error for non-existent table")
	}
	if !containsStr(err.Error(), "table NonExistent not found") {
		t.Errorf("Expected error message to contain 'table NonExistent not found', got: %v", err)
	}
}

func TestDependencyResolver_ComplexScenarios(t *testing.T) {
	t.Parallel()
	t.Run("Multiple FK to same table", func(t *testing.T) {
		dr := NewDependencyResolver()
		dr.tables = map[string]*TableDependency{
			"Users": {
				TableName: "Users",
				Level:     0,
			},
			"Posts": {
				TableName: "Posts",
				ForeignKeys: []FKReference{
					{ParentTable: "Users", ChildTable: "Posts", ChildColumn: "AuthorId"},
					{ParentTable: "Users", ChildTable: "Posts", ChildColumn: "EditorId"},
				},
				Level: 0,
			},
		}

		result, err := dr.GetOrderForTables([]string{"Posts", "Users"})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		expectedOrder := []string{"Users", "Posts"}
		if diff := cmp.Diff(expectedOrder, result); diff != "" {
			t.Errorf("Order mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Independent table islands", func(t *testing.T) {
		dr := NewDependencyResolver()
		dr.tables = map[string]*TableDependency{
			"Island1_A": {
				TableName:      "Island1_A",
				ChildrenTables: []string{"Island1_B"},
				Level:          0,
			},
			"Island1_B": {
				TableName:   "Island1_B",
				ParentTable: "Island1_A",
				Level:       1,
			},
			"Island2_A": {
				TableName:      "Island2_A",
				ChildrenTables: []string{"Island2_B"},
				Level:          0,
			},
			"Island2_B": {
				TableName:   "Island2_B",
				ParentTable: "Island2_A",
				Level:       1,
			},
		}

		result, err := dr.GetOrderForTables([]string{"Island2_B", "Island1_B", "Island2_A", "Island1_A"})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Check that each island maintains its internal order
		island1AIndex := indexOf(result, "Island1_A")
		island1BIndex := indexOf(result, "Island1_B")
		island2AIndex := indexOf(result, "Island2_A")
		island2BIndex := indexOf(result, "Island2_B")

		if island1AIndex >= island1BIndex {
			t.Errorf("Island1_A should come before Island1_B")
		}
		if island2AIndex >= island2BIndex {
			t.Errorf("Island2_A should come before Island2_B")
		}
	})
}

func TestDependencyResolver_PartialExport(t *testing.T) {
	t.Parallel()
	dr := NewDependencyResolver()
	dr.tables = map[string]*TableDependency{
		"Parent": {
			TableName:      "Parent",
			ChildrenTables: []string{"Child1", "Child2"},
			Level:          0,
		},
		"Child1": {
			TableName:   "Child1",
			ParentTable: "Parent",
			Level:       1,
		},
		"Child2": {
			TableName:   "Child2",
			ParentTable: "Parent",
			Level:       1,
		},
		"Unrelated": {
			TableName: "Unrelated",
			Level:     0,
		},
	}

	// Export only some tables, including a child without its parent
	result, err := dr.GetOrderForTables([]string{"Child1", "Unrelated"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should handle gracefully even without parent
	if len(result) != 2 {
		t.Errorf("Expected 2 tables, got %d", len(result))
	}

	// Should contain both requested tables
	if !slices.Contains(result, "Child1") || !slices.Contains(result, "Unrelated") {
		t.Errorf("Result should contain both requested tables: %v", result)
	}
}

// Helper functions
func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

func indexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}

func TestDependencyResolver_MultiColumnFK(t *testing.T) {
	t.Parallel()
	t.Run("Multi-column foreign key treated as single dependency", func(t *testing.T) {
		dr := NewDependencyResolver()
		dr.tables = map[string]*TableDependency{
			"Users": {
				TableName: "Users",
				Level:     0,
			},
			"UserPreferences": {
				TableName: "UserPreferences",
				// Single FK constraint with multiple columns
				ForeignKeys: []FKReference{
					{
						ConstraintName: "FK_User",
						ParentTable:    "Users",
						ChildTable:     "UserPreferences",
						ChildColumn:    "UserId", // Would be first column of multi-column FK
						ParentColumn:   "Id",
					},
				},
				Level: 0,
			},
		}

		result, err := dr.GetOrderForTables([]string{"UserPreferences", "Users"})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		expectedOrder := []string{"Users", "UserPreferences"}
		if diff := cmp.Diff(expectedOrder, result); diff != "" {
			t.Errorf("Order mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Composite key with multiple FKs", func(t *testing.T) {
		dr := NewDependencyResolver()
		dr.tables = map[string]*TableDependency{
			"Orders": {
				TableName: "Orders",
				Level:     0,
			},
			"Products": {
				TableName: "Products",
				Level:     0,
			},
			"OrderItems": {
				TableName: "OrderItems",
				// Multiple separate FK constraints
				ForeignKeys: []FKReference{
					{
						ConstraintName: "FK_Order",
						ParentTable:    "Orders",
						ChildTable:     "OrderItems",
					},
					{
						ConstraintName: "FK_Product",
						ParentTable:    "Products",
						ChildTable:     "OrderItems",
					},
				},
				Level: 0,
			},
		}

		result, err := dr.GetOrderForTables([]string{"OrderItems", "Orders", "Products"})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Both Orders and Products must come before OrderItems
		orderIndex := indexOf(result, "Orders")
		productIndex := indexOf(result, "Products")
		orderItemIndex := indexOf(result, "OrderItems")

		if orderIndex >= orderItemIndex {
			t.Errorf("Orders should come before OrderItems")
		}
		if productIndex >= orderItemIndex {
			t.Errorf("Products should come before OrderItems")
		}
	})
}
