// Copyright 2026 apstndb
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mycli

import "testing"

func TestEnumVarSetInvalidValueListsValuesDeterministically(t *testing.T) {
	t.Parallel()

	value := "alpha"
	variable := &EnumVar[string]{
		ptr: &value,
		values: map[string]string{
			"BETA":  "beta",
			"ALPHA": "alpha",
		},
	}

	err := variable.Set("unknown")
	if err == nil {
		t.Fatal("Set(unknown) error = nil, want error")
	}
	if got, want := err.Error(), "invalid value \"unknown\", must be one of: ALPHA, BETA"; got != want {
		t.Errorf("Set(unknown) error = %q, want %q", got, want)
	}
}

func TestProtoEnumVarSetRejectsNumericSuffix(t *testing.T) {
	t.Parallel()

	value := int32(2)
	variable := &ProtoEnumVar[int32]{
		ptr: &value,
		values: map[string]int32{
			"ALPHA": 1,
			"BETA":  2,
		},
	}

	err := variable.Set("1garbage")
	if err == nil {
		t.Fatal("Set(1garbage) error = nil, want error")
	}
	if got, want := err.Error(), "invalid value \"1garbage\", must be one of: ALPHA, BETA"; got != want {
		t.Errorf("Set(1garbage) error = %q, want %q", got, want)
	}
	if value != 2 {
		t.Errorf("value = %d, want 2", value)
	}
}

func TestProtoEnumVarSetAcceptsFullNumericValue(t *testing.T) {
	t.Parallel()

	value := int32(1)
	variable := &ProtoEnumVar[int32]{
		ptr: &value,
		values: map[string]int32{
			"ALPHA": 1,
			"BETA":  2,
		},
	}

	if err := variable.Set("2"); err != nil {
		t.Fatalf("Set(2) error = %v, want nil", err)
	}
	if value != 2 {
		t.Errorf("value = %d, want 2", value)
	}
}

func TestProtoEnumVarSetInvalidValueListsValuesDeterministically(t *testing.T) {
	t.Parallel()

	value := int32(1)
	variable := &ProtoEnumVar[int32]{
		ptr: &value,
		values: map[string]int32{
			"BETA":  2,
			"ALPHA": 1,
		},
	}

	err := variable.Set("unknown")
	if err == nil {
		t.Fatal("Set(unknown) error = nil, want error")
	}
	if got, want := err.Error(), "invalid value \"unknown\", must be one of: ALPHA, BETA"; got != want {
		t.Errorf("Set(unknown) error = %q, want %q", got, want)
	}
}
