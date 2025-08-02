package main

import (
	"reflect"

	"github.com/apstndb/spanner-mycli/tools/sysvargen"
)

// SysVarInfo contains parsed system variable information from struct tags
type SysVarInfo struct {
	FieldName   string
	Name        string
	Description string
	ReadOnly    bool
	Setter      string
	Getter      string
	Type        string
}

// readSysVarTags reads the sysvar tags from the systemVariables struct
// and returns a map of field name to SysVarInfo
func readSysVarTags() map[string]SysVarInfo {
	sv := systemVariables{}
	t := reflect.TypeOf(sv)
	result := make(map[string]SysVarInfo)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("sysvar")
		if tag == "" || tag == "-" {
			continue
		}

		parsedTag, err := sysvargen.ParseSysVarTag(tag)
		if err != nil || parsedTag.Name == "" {
			continue
		}

		info := SysVarInfo{
			FieldName:   field.Name,
			Name:        parsedTag.Name,
			Description: parsedTag.Description,
			ReadOnly:    parsedTag.ReadOnly,
			Setter:      parsedTag.Setter,
			Getter:      parsedTag.Getter,
			Type:        parsedTag.Type,
		}

		result[field.Name] = info
	}

	return result
}

// getSysVarInfo returns the system variable info for a specific field
// This can be used in manual registration to get name and description
func getSysVarInfo(fieldName string) (SysVarInfo, bool) {
	// Cache the tags on first call
	if sysVarTagCache == nil {
		sysVarTagCache = readSysVarTags()
	}
	info, ok := sysVarTagCache[fieldName]
	return info, ok
}

var sysVarTagCache map[string]SysVarInfo
