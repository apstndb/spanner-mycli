//
// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spantype"
	"github.com/apstndb/spanvalue"
)

func DecodeRow(row *spanner.Row) ([]string, error) {
	return spanvalue.FormatRowSpannerCLICompatible(row)
}

func DecodeColumn(column spanner.GenericColumnValue) (string, error) {
	return spanvalue.FormatColumnSpannerCLICompatible(column)
}

// formatTypeSimple is format type for headers.
func formatTypeSimple(typ *sppb.Type) string {
	return spantype.FormatType(typ, spantype.FormatOption{
		Struct: spantype.StructModeBase,
		Proto:  spantype.ProtoEnumModeBase,
		Enum:   spantype.ProtoEnumModeBase,
		Array:  spantype.ArrayModeRecursive,
	})
}

// formatTypeVerbose is format type for DESCRIBE.
func formatTypeVerbose(typ *sppb.Type) string {
	return spantype.FormatTypeMoreVerbose(typ)
}
