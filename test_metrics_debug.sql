-- Enable client metrics collection
SET CLI_ENABLE_CLIENT_METRICS = true;

-- Check current value
\echo CLI_ENABLE_CLIENT_METRICS is set

-- Enable verbose output
SET CLI_VERBOSE = true;

-- Execute a query with timing
\timing on

-- Execute a simple query
SELECT 1 AS test_column;

-- Try to see if metrics are collected
SELECT 'Metrics Test' AS description;