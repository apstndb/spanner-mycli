// Copyright 2026 apstndb
// SPDX-License-Identifier: Apache-2.0

package mycli

import (
	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanvalue"
)

// parameterDisplayValue omits type annotations because SHOW PARAM reports the
// complete type separately. This is display notation, not a SQL expression.
// Strings stay quoted to distinguish the string 'NULL' from SQL NULL.
func parameterDisplayValue(value spanner.GenericColumnValue) (string, error) {
	literal := spanvalue.LiteralFormatConfigWithSingleQuotedLiterals()
	fc := spanvalue.SimpleFormatConfig().WithComplexPlugin(
		spanvalue.PluginForStruct(spanvalue.FormatSimpleStructField, spanvalue.FormatTupleStruct),
	).WithComplexPlugin(func(_ spanvalue.Formatter, v spanner.GenericColumnValue, top bool) (string, error) {
		switch v.Type.GetCode() {
		case sppb.TypeCode_STRING, sppb.TypeCode_BYTES:
			return literal.FormatColumn(v, top)
		default:
			return "", spanvalue.ErrFallthrough
		}
	})
	fc.NullString = "NULL"
	return fc.FormatToplevelColumn(value)
}
