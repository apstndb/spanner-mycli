import re

file_path = "/Users/apstndb/work/apstndb/spanner-mycli/.git/phantom/worktrees/issues/446/integration_test.go"

# Read the file
with open(file_path, 'r') as f:
    lines = f.readlines()

# Test cases to comment (line numbers minus 1 for 0-based indexing)
test_cases = [
    (462, "SHOW VARIABLE CLI_VERSION"),
    (679, "HELP"),
    (687, "SHOW DDLS"),
    (705, "SPLIT POINTS statements"),
    (709, "EXPLAIN & EXPLAIN ANALYZE statements"),
    (732, "DATABASE statements (dedicated instance)"),
    (762, "SHOW TABLES"),
    (773, "TRY PARTITIONED QUERY"),
    (785, "CLI_TRY_PARTITION_QUERY system variable"),
    (826, "mutation, pdml, partitioned query"),
    (960, "SHOW VARIABLES"),
    (977, "HELP VARIABLES"),
    (1009, "CQL SELECT"),
    (1013, "SET ADD statement for CLI_PROTO_FILES"),
    (1036, "SHOW CREATE INDEX"),
    (1051, "DESCRIBE DML (INSERT with literal)"),
    (1064, "PARTITION SELECT query"),
    (1081, "SHOW SCHEMA UPDATE OPERATIONS (empty result expected)"),
    (1092, "MUTATE DELETE"),
]

# Process each test case
for line_num, desc in test_cases:
    # Find the closing brace for this test case
    depth = 0
    start_idx = line_num
    end_idx = line_num
    
    for i in range(line_num, len(lines)):
        for char in lines[i]:
            if char == '{':
                depth += 1
            elif char == '}':
                depth -= 1
                if depth == 0:
                    end_idx = i
                    break
        if depth == 0:
            break
    
    # Add comment markers
    lines[line_num] = f"\t\t// Moved to TestMiscStatements\n\t\t/*{{\n"
    lines[end_idx] = lines[end_idx].rstrip() + "*/\n"

# Write back
with open(file_path, 'w') as f:
    f.writelines(lines)

print("Successfully commented out all remaining test cases")