package main

import (
	"github.com/apstndb/memebridge"
	"github.com/apstndb/spanvalue/gcvctor"
	"github.com/cloudspannerecosystem/memefish/ast"
)

func generateParams(paramsNodeMap map[string]ast.Node, includeType bool) (map[string]any, error) {
	result := make(map[string]any)
	for k, v := range paramsNodeMap {
		switch v := v.(type) {
		case ast.Type:
			if !includeType {
				continue
			}

			typ, err := memebridge.MemefishTypeToSpannerpbType(v)
			if err != nil {
				return nil, err
			}
			nullValue := gcvctor.TypedNull(typ)
			if err != nil {
				return nil, err
			}
			result[k] = nullValue
		case ast.Expr:
			expr, err := memebridge.MemefishExprToGCV(v)
			if err != nil {
				return nil, err
			}
			result[k] = expr
		}
	}
	return result, nil
}
