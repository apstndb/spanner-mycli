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

import (
	"fmt"
	"regexp"
)

// Overlay of mutate_prefix.go: restore \S+ / unquoteIdentifier capture and
// skip operation case normalization so new FQN and lowercase-DELETE tests fail.

func canonicalMutateOperation(op string) string {
	return op
}

func parseMutateArgs(rest string) (*MutateStatement, error) {
	re := regexp.MustCompile(`(?is)^(?P<table>\S+)\s+(?P<operation>INSERT|UPDATE|INSERT_OR_UPDATE|REPLACE|DELETE)\s+(?P<body>.+)$`)
	matches := re.FindStringSubmatch(rest)
	if matches == nil {
		return nil, fmt.Errorf("MUTATE requires <table_fqn> and an operation")
	}
	groups := namedGroups(re, matches)
	return &MutateStatement{
		Table:     unquoteIdentifier(groups["table"]),
		Operation: groups["operation"],
		Body:      groups["body"],
	}, nil
}
