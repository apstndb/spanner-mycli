#!/bin/bash

export SPANNER_PROJECT_ID=gcpug-public-spanner
export SPANNER_INSTANCE_ID=merpay-sponsored-instance
export SPANNER_DATABASE_ID=apstndb-sampledb-with-data-idx

echo "=== Performance Test: Full Songs Table (1,024,000 rows) ==="
echo ""

# Test with streaming (AUTO mode with CSV will stream)
echo "Testing with streaming mode (CSV):"
time echo "SET CLI_STREAMING = AUTO; SET CLI_FORMAT = CSV; SELECT SingerId, AlbumId, TrackId, SongName, Duration, SongGenre FROM Songs;" | \
    ./spanner-mycli --log-level=DEBUG 2>&1 | \
    grep -E "(Memory usage:|TTFB:|Rows processed:)" | head -10

echo ""
echo "---"
echo ""

# Test without streaming (AUTO mode with TABLE will buffer)
echo "Testing with buffered mode (TABLE):"
time echo "SET CLI_STREAMING = AUTO; SET CLI_FORMAT = TABLE; SELECT SingerId, AlbumId, TrackId, SongName, Duration, SongGenre FROM Songs LIMIT 100000;" | \
    ./spanner-mycli --log-level=DEBUG 2>&1 | \
    grep -E "(Memory usage:|TTFB:|Rows processed:)" | head -10

echo ""
echo "=== Performance Test Complete ==="