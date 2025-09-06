# Sample Database Metadata Format

This directory contains embedded sample databases for spanner-mycli. You can also create your own custom sample databases by providing metadata files.

## Metadata File Format

Sample database metadata files can be in JSON or YAML format and describe the location and properties of your sample database. Here's the structure:

```json
{
  "name": "sample-name",
  "description": "A brief description of the sample database",
  "dialect": "GOOGLE_STANDARD_SQL",
  "schemaURI": "path/to/schema.sql",
  "dataURI": "path/to/data.sql",
  "source": "URL or description of the source"
}
```

### Field Descriptions

- **name** (required): Unique identifier for the sample
- **description** (required): Human-readable description  
- **dialect** (required): Either `"GOOGLE_STANDARD_SQL"` or `"POSTGRESQL"`
- **schemaURI** (required): Path or URI to the DDL schema file
- **dataURI** (optional): Path or URI to the data file with INSERT statements
- **source** (optional): Documentation URL or description of the source

### URI Support

The `schemaURI` and `dataURI` fields support multiple URI schemes:

- **Relative paths**: Resolved relative to the metadata file location
  - Example: `"schema.sql"` looks for the file in the same directory as metadata.json
- **file://** - Local file system absolute paths
  - Example: `"file:///home/user/samples/schema.sql"`
- **gs://** - Google Cloud Storage
  - Example: `"gs://my-bucket/samples/schema.sql"`
- **https://** or **http://** - Web URLs
  - Example: `"https://raw.githubusercontent.com/myorg/samples/main/schema.sql"`

## Usage Examples

### Using Built-in Samples

```bash
# List all available built-in samples
spanner-mycli --list-samples

# Use a built-in sample
spanner-mycli --embedded-emulator --sample-database=fingraph
spanner-mycli --embedded-emulator --sample-database=singers
```

### Creating Custom Samples

1. Create a directory for your sample:
```bash
mkdir mysample
cd mysample
```

2. Create your schema file (`schema.sql`):
```sql
CREATE TABLE Users (
  UserId INT64 NOT NULL,
  UserName STRING(100),
  Email STRING(100),
) PRIMARY KEY (UserId);
```

3. Create your data file (`data.sql`):
```sql
INSERT INTO Users (UserId, UserName, Email) VALUES
  (1, 'Alice', 'alice@example.com'),
  (2, 'Bob', 'bob@example.com');
```

4. Create the metadata file (`metadata.json`):
```json
{
  "name": "mysample",
  "description": "My custom sample database",
  "dialect": "GOOGLE_STANDARD_SQL",
  "schemaURI": "schema.sql",
  "dataURI": "data.sql"
}
```

5. Use your custom sample:
```bash
spanner-mycli --embedded-emulator --sample-database=/path/to/mysample/metadata.json
```

### Advanced Example with Mixed Sources

You can mix different URI schemes in a single metadata file:

```json
{
  "name": "production-snapshot",
  "description": "Production database snapshot with test data",
  "dialect": "POSTGRESQL",
  "schemaURI": "https://raw.githubusercontent.com/myorg/schemas/main/prod.sql",
  "dataURI": "gs://my-test-data/prod-sample.sql",
  "source": "Internal production system"
}
```

## Built-in Samples

### fingraph
Financial graph database demonstrating Spanner Graph features with Person, Account, and Transfer relationships.

### singers
Music database used throughout Spanner documentation with Singers, Albums, Songs, and Concerts tables demonstrating interleaved table relationships.

## File Size Limits

Sample database files are limited to 10MB per file for security and performance reasons. This should be sufficient for sample data. If you need larger datasets, consider using a subset of data or linking to external sources.