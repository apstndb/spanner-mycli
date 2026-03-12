GoogleSQL for Spanner supports the following sequence functions.

## Function list

<table>
<thead>
<tr class="header">
<th>Name</th>
<th>Summary</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/sequence_functions#get_internal_sequence_state"><code dir="ltr" translate="no">        GET_INTERNAL_SEQUENCE_STATE       </code></a></td>
<td>Gets the current sequence internal counter before bit reversal.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/sequence_functions#get_next_sequence_value"><code dir="ltr" translate="no">        GET_NEXT_SEQUENCE_VALUE       </code></a></td>
<td>Takes in a sequence identifier and returns the next value. This function is only allowed in read-write transactions.</td>
</tr>
</tbody>
</table>

## `     GET_INTERNAL_SEQUENCE_STATE    `

``` text
GET_INTERNAL_SEQUENCE_STATE(SEQUENCE sequence_identifier)
```

**Description**

Gets the current sequence internal counter before bit reversal. This function is useful for import or export, and migrations. If `  GET_NEXT_SEQUENCE_VALUE  ` is never called on the sequence, then this function returns `  NULL  ` .

**Arguments**

  - `  sequence_identifier  ` : The ID for the sequence.

**Return Data Type**

`  INT64  `

**Example**

``` text
SELECT GET_NEXT_SEQUENCE_VALUE(SEQUENCE MySequence) AS next_value;

/*---------------------+
 | next_value          |
 +---------------------+
 | 5980780305148018688 |
 +---------------------*/
```

``` text
SELECT GET_INTERNAL_SEQUENCE_STATE(SEQUENCE MySequence) AS sequence_state;

/*----------------+
 | sequence_state |
 +----------------+
 | 399            |
 +----------------*/
```

## `     GET_NEXT_SEQUENCE_VALUE    `

``` text
GET_NEXT_SEQUENCE_VALUE(SEQUENCE sequence_identifier)
```

**Description**

Gets the next integer in a sequence.

**Arguments**

  - `  sequence_identifier  ` : The ID for the sequence.

**Return Data Type**

`  INT64  `

**Example**

Create a table where its key column uses the sequence as a default value.

``` text
CREATE TABLE Singers (
  SingerId INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(SEQUENCE MySequence)),
  a STRING(MAX),
) PRIMARY KEY (SingerId);
```

Obtain a sequence value in a read-write transaction and use it in an INSERT statement.

``` text
SELECT GET_NEXT_SEQUENCE_VALUE(SEQUENCE MySequence) as next_id;
INSERT INTO Singers(SingerId, a) VALUES (next_id, 1);
```

Use the sequence functions independently in the GoogleSQL DML.

``` text
INSERT INTO Singers (SingerId) VALUES (GET_NEXT_SEQUENCE_VALUE(SEQUENCE MySequence);
```
