# Spanner Client-Side Metrics Implementation Summary

## What's Implemented

This implementation adds support for collecting and displaying Spanner client-side metrics in spanner-mycli using OpenTelemetry.

### Features Added

1. **System Variable**: `CLI_ENABLE_CLIENT_METRICS` - Enables client-side metrics collection
2. **Metrics Collector**: In-memory OpenTelemetry metrics collection without external dependencies
3. **Display Integration**: Metrics are displayed in verbose output after query results

### Available Metrics

Currently, the following metrics are collected by the Spanner Go client v1.83.0:

- **spanner/gfe_latency**: Google Front End latency (time between Google network receiving RPC and first response byte)
  - Displayed as "gfe latency" in milliseconds
  - Properly filtered to show metrics for the actual query (not session creation)
  
Additional session-related metrics available:
- spanner/open_session_count
- spanner/max_allowed_sessions
- spanner/num_sessions_in_pool
- spanner/max_in_use_sessions
- spanner/num_acquired_sessions
- spanner/num_released_sessions

### Metrics Structure

The implementation includes placeholders for additional metrics that may become available in future versions:
- AFE latency (Application Front End - only available with direct access in Google Cloud)
- Operation latency
- Attempt latency
- Attempt count
- GFE/AFE error counts

### Usage

Enable metrics at startup:
```bash
spanner-mycli --set CLI_ENABLE_CLIENT_METRICS=true --set CLI_VERBOSE=true
```

Or set interactively (requires reconnection):
```sql
SET CLI_ENABLE_CLIENT_METRICS = true;
SET CLI_VERBOSE = true;
-- Reconnect to apply metrics collection
\connect
```

### Technical Details

- Uses OpenTelemetry's ManualReader for in-memory metrics collection
- Filters metrics by gRPC method to show relevant query metrics
- Converts microseconds to milliseconds for display
- Only displays metrics that have non-zero values

### Known Limitations

1. AFE metrics require direct access (GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS=true) which only works within Google Cloud
2. Operation/attempt latency metrics documented in Cloud Spanner docs are not yet exposed via OpenTelemetry in the Go client v1.83.0
   - The metrics exist internally (as seen in metrics.go) but are not yet available through the OpenTelemetry interface
   - Only GFE latency and session metrics are currently exposed
3. Metrics collection must be enabled before session creation
4. The `spanner.googleapis.com/client/` prefix used in documentation is added during export; internally metrics use simpler names

### Future Improvements

As the Spanner Go client adds more metrics, they will automatically be collected and can be displayed by updating the metric name mappings in `metrics_collector.go`.