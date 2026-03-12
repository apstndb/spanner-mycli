This section describes stored system procedures for Spanner.

A stored system procedure contains SQL code that you can reuse. Spanner provides stored system procedures for you to use. You can't create your own stored procedure in Spanner. You can only execute one stored procedure at a time in a `  CALL  ` statement.

## Stored system procedures

To execute a stored system procedure, you use the [`  CALL  `](/spanner/docs/reference/standard-sql/procedural-language#call) statement:

``` text
CALL procedure_name(parameters);
```

Replace procedure\_name with the name of the stored system procedure.

Spanner supports the following stored system procedures:

  - [Query cancellation](#query-cancellation)

### Query cancellation

This section describes the query cancellation stored system procedure.

#### Syntax

Cancels a query with the specified ***query\_id*** .

``` text
CALL cancel_query(query_id)
```

#### Description

This stored system procedure has the following parameters:

<table>
<thead>
<tr class="header">
<th>Parameter</th>
<th>Type</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       query_id      </code></td>
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td>Specifies the ID for the query that you want to cancel.</td>
</tr>
</tbody>
</table>

Query cancellations might fail in the following circumstances:

  - When Spanner servers are busy due to heavy query loads.
  - When the query is in the [process of restarting](/spanner/docs/introspection/oldest-active-queries#limitations) due to an error.

In both cases, you can run the query cancellation stored system procedureagain.
