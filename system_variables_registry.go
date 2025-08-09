package main

import (
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"time"

	"github.com/apstndb/spanner-mycli/internal/parser/sysvar"
	"google.golang.org/protobuf/types/descriptorpb"
)

// createSystemVariableRegistry creates and configures the parser registry for system variables.
// This function sets up all the system variable parsers with their getters and setters.
func createSystemVariableRegistry(sv *systemVariables) *sysvar.Registry {
	registry := sysvar.NewRegistry()

	// Register most variables using the generated code
	registerGeneratedVariables(registry, sv)

	// Register variables that need custom handling
	registerCustomVariables(registry, sv)

	return registry
}

// createUnimplementedParser creates a parser that returns the legacy error types
// for compatibility with existing tests and error handling.
func createUnimplementedParser(name, description string) sysvar.VariableParser {
	// Use the built-in unimplemented parser
	return sysvar.NewUnimplementedVariableParser(name, description)
}

// registerCustomVariables registers variables that need custom parsers or complex logic
// that cannot be automatically generated from struct tags.
func registerCustomVariables(registry *sysvar.Registry, sv *systemVariables) {
	// Variables that need custom parsers or complex logic

	// READ_ONLY_STALENESS - complex type with special formatting
	mustRegister(registry, sysvar.NewStringVariableParser(
		"READ_ONLY_STALENESS",
		"A property of type STRING for read-only transactions with flexible staleness.",
		func() string {
			if sv.ReadOnlyStaleness == nil {
				return ""
			}
			// Format the staleness value similar to the old system
			s := sv.ReadOnlyStaleness.String()
			stalenessRe := regexp.MustCompile(`^\(([^:]+)(?:: (.+))?\)$`)
			matches := stalenessRe.FindStringSubmatch(s)
			if matches == nil {
				return s
			}
			switch matches[1] {
			case "strong":
				return "STRONG"
			case "exactStaleness":
				return fmt.Sprintf("EXACT_STALENESS %v", matches[2])
			case "maxStaleness":
				return fmt.Sprintf("MAX_STALENESS %v", matches[2])
			case "readTimestamp":
				ts, err := parseTimeString(matches[2])
				if err != nil {
					return s
				}
				return fmt.Sprintf("READ_TIMESTAMP %v", ts.Format(time.RFC3339Nano))
			case "minReadTimestamp":
				ts, err := parseTimeString(matches[2])
				if err != nil {
					return s
				}
				return fmt.Sprintf("MIN_READ_TIMESTAMP %v", ts.Format(time.RFC3339Nano))
			default:
				return s
			}
		},
		func(v string) error {
			if v == "" {
				sv.ReadOnlyStaleness = nil
				return nil
			}
			// Use unquoteString for compatibility with the old system
			staleness, err := parseTimestampBound(unquoteString(v))
			if err != nil {
				return err
			}
			sv.ReadOnlyStaleness = &staleness
			return nil
		},
	))

	// READ_TIMESTAMP - handled in generated code

	// COMMIT_RESPONSE - complex type
	// Skipping as it's not in the original registry

	// COMMIT_TIMESTAMP - handled in generated code

	// AUTOCOMMIT_DML_MODE - custom enum
	autocommitDMLModeValues := map[string]AutocommitDMLMode{
		"TRANSACTIONAL":          AutocommitDMLModeTransactional,
		"PARTITIONED_NON_ATOMIC": AutocommitDMLModePartitionedNonAtomic,
	}
	autocommitInfo, _ := getSysVarInfo("AutocommitDMLMode")
	mustRegister(registry, sysvar.NewEnumVariableParser(
		autocommitInfo.Name,
		autocommitInfo.Description,
		autocommitDMLModeValues,
		sysvar.GetValue(&sv.AutocommitDMLMode),
		sysvar.SetValue(&sv.AutocommitDMLMode),
		sysvar.FormatEnumFromMap("AutocommitDMLMode", autocommitDMLModeValues),
	))

	// CLI_FORMAT - custom enum (DisplayMode)
	formatValues := map[string]DisplayMode{
		"TABLE":                DisplayModeTable,
		"TABLE_COMMENT":        DisplayModeTableComment,
		"TABLE_DETAIL_COMMENT": DisplayModeTableDetailComment,
		"VERTICAL":             DisplayModeVertical,
		"TAB":                  DisplayModeTab,
		"HTML":                 DisplayModeHTML,
		"XML":                  DisplayModeXML,
		"CSV":                  DisplayModeCSV,
	}
	mustRegister(registry, sysvar.NewEnumVariableParser(
		"CLI_FORMAT",
		"Controls output format for query results. Valid values: TABLE (ASCII table), TABLE_COMMENT (table in comments), TABLE_DETAIL_COMMENT, VERTICAL (column:value pairs), TAB (tab-separated), HTML (HTML table), XML (XML format), CSV (comma-separated values).",
		formatValues,
		sysvar.GetValue(&sv.CLIFormat),
		sysvar.SetValue(&sv.CLIFormat),
		sysvar.FormatEnumFromMap("DisplayMode", formatValues),
	))

	// CLI_PROTO_DESCRIPTOR_FILE - complex proto descriptor file parser
	mustRegister(registry, sysvar.NewProtoDescriptorFileParser(
		"CLI_PROTO_DESCRIPTOR_FILE",
		"Comma-separated list of proto descriptor files. Supports ADD to append files.",
		sysvar.GetValue(&sv.ProtoDescriptorFile),
		func(files []string) error {
			// Set operation - replace all files
			if len(files) == 0 {
				sv.ProtoDescriptorFile = []string{}
				sv.ProtoDescriptor = nil
				return nil
			}

			var fileDescriptorSet *descriptorpb.FileDescriptorSet
			for _, filename := range files {
				fds, err := readFileDescriptorProtoFromFile(filename)
				if err != nil {
					return err
				}
				fileDescriptorSet = mergeFDS(fileDescriptorSet, fds)
			}

			sv.ProtoDescriptorFile = files
			sv.ProtoDescriptor = fileDescriptorSet
			return nil
		},
		func(filename string) error {
			// Add operation - append a file
			fds, err := readFileDescriptorProtoFromFile(filename)
			if err != nil {
				return err
			}

			if !slices.Contains(sv.ProtoDescriptorFile, filename) {
				sv.ProtoDescriptorFile = slices.Concat(sv.ProtoDescriptorFile, sliceOf(filename))
				sv.ProtoDescriptor = &descriptorpb.FileDescriptorSet{File: slices.Concat(sv.ProtoDescriptor.GetFile(), fds.GetFile())}
			} else {
				sv.ProtoDescriptor = mergeFDS(sv.ProtoDescriptor, fds)
			}
			return nil
		},
		nil, // No additional validation needed
	))

	// CLI_PARSE_MODE - custom enum
	mustRegister(registry, sysvar.NewSimpleEnumParser(
		"CLI_PARSE_MODE",
		"Controls statement parsing mode: FALLBACK (default), NO_MEMEFISH, MEMEFISH_ONLY, or UNSPECIFIED",
		sysvar.BuildEnumMapWithAliases(
			[]parseMode{
				parseModeUnspecified, // ""
				parseModeFallback,    // "FALLBACK"
				parseModeNoMemefish,  // "NO_MEMEFISH"
				parseMemefishOnly,    // "MEMEFISH_ONLY"
			},
			map[parseMode][]string{
				parseModeUnspecified: {"UNSPECIFIED"}, // Alias for ""
			},
		),
		sysvar.GetValue(&sv.BuildStatementMode),
		sysvar.SetValue(&sv.BuildStatementMode),
	))

	// CLI_LOG_LEVEL - enum with slog.Level
	logLevelValues := map[string]slog.Level{
		"DEBUG": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"WARN":  slog.LevelWarn,
		"ERROR": slog.LevelError,
	}
	mustRegister(registry, sysvar.NewEnumVariableParser(
		"CLI_LOG_LEVEL",
		"Log level for the CLI.",
		logLevelValues,
		sysvar.GetValue(&sv.LogLevel),
		sv.setLogLevel,
		slog.Level.String,
	))

	// CLI_ANALYZE_COLUMNS - custom template parser
	mustRegister(registry, sysvar.NewStringVariableParser(
		"CLI_ANALYZE_COLUMNS",
		"Go template for analyzing column data.",
		sysvar.GetValue(&sv.AnalyzeColumns),
		func(v string) error {
			def, err := customListToTableRenderDefs(v)
			if err != nil {
				return err
			}

			sv.AnalyzeColumns = v
			sv.ParsedAnalyzeColumns = def
			return nil
		},
	))

	// CLI_INLINE_STATS - custom template parser
	mustRegister(registry, sysvar.NewStringVariableParser(
		"CLI_INLINE_STATS",
		"<name>:<template>, ...",
		sysvar.GetValue(&sv.InlineStats),
		func(v string) error {
			defs, err := parseInlineStatsDefs(v)
			if err != nil {
				return err
			}

			sv.InlineStats = v
			sv.ParsedInlineStats = defs
			return nil
		},
	))

	// CLI_EXPLAIN_FORMAT - custom enum
	mustRegister(registry, sysvar.NewSimpleEnumParser(
		"CLI_EXPLAIN_FORMAT",
		"Controls query plan notation. CURRENT(default): new notation, TRADITIONAL: spanner-cli compatible notation, COMPACT: compact notation.",
		sysvar.SliceToEnumMap([]explainFormat{
			explainFormatUnspecified,
			explainFormatCurrent,
			explainFormatTraditional,
			explainFormatCompact,
		}),
		sysvar.GetValue(&sv.ExplainFormat),
		sysvar.SetValue(&sv.ExplainFormat),
	))

	// CLI_DIRECTED_READ - complex type
	// Skipping as it's not in the original registry
}

// mustRegister registers a variable parser with the registry and panics on error.
// This is used during initialization where registration failures are fatal.
func mustRegister(registry *sysvar.Registry, parser sysvar.VariableParser) {
	if err := registry.Register(parser); err != nil {
		panic(fmt.Sprintf("Failed to register %s: %v", parser.Name(), err))
	}
}
