-- Enable client metrics collection
SET CLI_ENABLE_CLIENT_METRICS = true;

-- Enable verbose output to see metrics
SET CLI_VERBOSE = true;

-- Execute a simple query
SELECT 1 AS test_column;

-- Execute another query to see metrics
SELECT 'Hello, Metrics!' AS greeting;

-- Try a slightly more complex query
SELECT 1 AS id, 'Test' AS name
UNION ALL
SELECT 2, 'Metrics';