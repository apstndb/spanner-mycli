package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cloudspannerecosystem/memefish/ast"
	"github.com/ngicks/go-iterator-helper/x/exp/xiter"
	xiter2 "spheric.cloud/xiter"
)

func toNamedType(fullName string) *ast.NamedType {
	return &ast.NamedType{Path: slices.Collect(xiter.Map(
		func(s string) *ast.Ident {
			return &ast.Ident{Name: s}
		},
		slices.Values(strings.Split(fullName, ".")))),
	}
}

func toNamedTypes(fullNames []string) []*ast.NamedType {
	return slices.Collect(xiter.Map(toNamedType, slices.Values(fullNames)))
}

func exprToFullName(expr ast.Expr) (string, error) {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name, nil
	case *ast.Path:
		return xiter2.Join(xiter.Map(func(ident *ast.Ident) string { return ident.Name }, slices.Values(e.Idents)), "."), nil
	default:
		return "", fmt.Errorf("must be ident or path, but: %T", expr)
	}
}
