GoogleSQL for Spanner supports the following GQL functions:

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
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#destination_node_id"><code dir="ltr" translate="no">        DESTINATION_NODE_ID       </code></a></td>
<td>Gets a unique identifier of a graph edge's destination node.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#edges"><code dir="ltr" translate="no">        EDGES       </code></a></td>
<td>Gets the edges in a graph path. The resulting array retains the original order in the graph path.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#element_definition_name"><code dir="ltr" translate="no">        ELEMENT_DEFINITION_NAME       </code></a></td>
<td>Gets a graph element's element definition name.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#element_id"><code dir="ltr" translate="no">        ELEMENT_ID       </code></a></td>
<td>Gets a graph element's unique identifier.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#is_acyclic"><code dir="ltr" translate="no">        IS_ACYCLIC       </code></a></td>
<td>Checks if a graph path has a repeating node.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#is_first"><code dir="ltr" translate="no">        IS_FIRST       </code></a></td>
<td>Returns <code dir="ltr" translate="no">       true      </code> if this row is in the first <code dir="ltr" translate="no">       k      </code> rows (1-based) within the window.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#is_simple"><code dir="ltr" translate="no">        IS_SIMPLE       </code></a></td>
<td>Checks if a graph path is simple.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#is_trail"><code dir="ltr" translate="no">        IS_TRAIL       </code></a></td>
<td>Checks if a graph path has a repeating edge.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#labels"><code dir="ltr" translate="no">        LABELS       </code></a></td>
<td>Gets the labels associated with a graph element.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#nodes"><code dir="ltr" translate="no">        NODES       </code></a></td>
<td>Gets the nodes in a graph path. The resulting array retains the original order in the graph path.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#path"><code dir="ltr" translate="no">        PATH       </code></a></td>
<td>Creates a graph path from a list of graph elements.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#path_first"><code dir="ltr" translate="no">        PATH_FIRST       </code></a></td>
<td>Gets the first node in a graph path.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#path_last"><code dir="ltr" translate="no">        PATH_LAST       </code></a></td>
<td>Gets the last node in a graph path.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#path_length"><code dir="ltr" translate="no">        PATH_LENGTH       </code></a></td>
<td>Gets the number of edges in a graph path.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#property_names"><code dir="ltr" translate="no">        PROPERTY_NAMES       </code></a></td>
<td>Gets the property names associated with a graph element.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#source_node_id"><code dir="ltr" translate="no">        SOURCE_NODE_ID       </code></a></td>
<td>Gets a unique identifier of a graph edge's source node.</td>
</tr>
</tbody>
</table>

## `     DESTINATION_NODE_ID    `

``` text
DESTINATION_NODE_ID(edge_element)
```

**Description**

Gets a unique identifier of a graph edge's destination node. The unique identifier is only valid for the scope of the query where it's obtained.

**Arguments**

  - `  edge_element  ` : A `  GRAPH_ELEMENT  ` value that represents an edge.

**Details**

Returns `  NULL  ` if `  edge_element  ` is `  NULL  ` .

**Return type**

`  STRING  `

**Examples**

``` text
GRAPH FinGraph
MATCH (:Person)-[o:Owns]->(a:Account)
RETURN a.id AS account_id, DESTINATION_NODE_ID(o) AS destination_node_id

/*------------------------------------------+
 |account_id | destination_node_id          |
 +-----------|------------------------------+
 | 7         | mUZpbkdyYXBoLkFjY291bnQAeJEO |
 | 16        | mUZpbkdyYXBoLkFjY291bnQAeJEg |
 | 20        | mUZpbkdyYXBoLkFjY291bnQAeJEo |
 +------------------------------------------*/
```

Note that the actual identifiers obtained may be different from what's shown above.

## `     EDGES    `

``` text
EDGES(graph_path)
```

**Description**

Gets the edges in a graph path. The resulting array retains the original order in the graph path.

**Definitions**

  - `  graph_path  ` : A `  GRAPH_PATH  ` value that represents a graph path.

**Details**

If `  graph_path  ` is `  NULL  ` , returns `  NULL  ` .

**Return type**

`  ARRAY<GRAPH_ELEMENT>  `

**Examples**

``` text
GRAPH FinGraph
MATCH p=(src:Account)-[t1:Transfers]->(mid:Account)-[t2:Transfers]->(dst:Account)
LET es = EDGES(p)
RETURN ARRAY_CONCAT(ARRAY_TRANSFORM(es, e -> e.Id), [dst.Id]) as ids_in_path

/*-------------+
 | ids_in_path |
 +-------------+
 | [16,20,7]   |
 +-------------+
 | [20,7,16]   |
 +-------------+
 | [20,7,16]   |
 +-------------+
 | [16,20,16]  |
 +-------------+
 | [7,16,20]   |
 +-------------+
 | [7,16,20]   |
 +-------------+
 | [20,16,20]  |
 +-------------*/
```

## `     ELEMENT_DEFINITION_NAME    `

``` text
ELEMENT_DEFINITION_NAME(element)
```

**Description**

Returns the name of the graph element table underlying the graph element.

**Arguments**

  - `  element  ` : A `  GRAPH_ELEMENT  ` value.

**Details**

Returns `  NULL  ` if `  element  ` is `  NULL  ` .

**Return type**

`  STRING  `

**Examples**

``` text
GRAPH FinGraph
MATCH (p:Person)-[o:Owns]->(:Account)
RETURN
  p.name AS name,
  ELEMENT_DEFINITION_NAME(p) AS node_element_definition_name,
  ELEMENT_DEFINITION_NAME(o) AS edge_element_definition_name

/*--------------------------------------------------------------------+
 | name | node_element_definition_name | edge_element_definition_name |
 +------|------------------------------|------------------------------+
 | Alex | Person                       | Owns                         |
 | Dana | Person                       | Owns                         |
 | Lee  | Person                       | Owns                         |
 +--------------------------------------------------------------------*/
```

## `     ELEMENT_ID    `

``` text
ELEMENT_ID(element)
```

**Description**

Gets a graph element's unique identifier. The unique identifier is only valid for the scope of the query where it's obtained.

**Arguments**

  - `  element  ` : A `  GRAPH_ELEMENT  ` value.

**Details**

Returns `  NULL  ` if `  element  ` is `  NULL  ` .

**Return type**

`  STRING  `

**Examples**

``` text
GRAPH FinGraph
MATCH (p:Person)-[o:Owns]->(:Account)
RETURN p.name AS name, ELEMENT_ID(p) AS node_element_id, ELEMENT_ID(o) AS edge_element_id

/*--------------------------------------------------------------------------------------------------------------------------------------------+
 | name | node_element_id              | edge_element_id                                                                                      |
 +------|------------------------------|------------------------------------------------------------------------------------------------------+
 | Alex | mUZpbkdyYXBoLlBlcnNvbgB4kQI= | mUZpbkdyYXBoLlBlcnNvbk93bkFjY291bnQAeJECkQ6ZRmluR3JhcGguUGVyc29uAHiRAplGaW5HcmFwaC5BY2NvdW50AHiRDg== |
 | Dana | mUZpbkdyYXBoLlBlcnNvbgB4kQQ= | mUZpbkdyYXBoLlBlcnNvbk93bkFjY291bnQAeJEGkSCZRmluR3JhcGguUGVyc29uAHiRBplGaW5HcmFwaC5BY2NvdW50AHiRIA== |
 | Lee  | mUZpbkdyYXBoLlBlcnNvbgB4kQY= | mUZpbkdyYXBoLlBlcnNvbk93bkFjY291bnQAeJEEkSiZRmluR3JhcGguUGVyc29uAHiRBJlGaW5HcmFwaC5BY2NvdW50AHiRKA== |
 +--------------------------------------------------------------------------------------------------------------------------------------------*/
```

Note that the actual identifiers obtained may be different from what's shown above.

## `     IS_ACYCLIC    `

``` text
IS_ACYCLIC(graph_path)
```

**Description**

Checks if a graph path has a repeating node. Returns `  TRUE  ` if a repetition isn't found, otherwise returns `  FALSE  ` .

**Definitions**

  - `  graph_path  ` : A `  GRAPH_PATH  ` value that represents a graph path.

**Details**

Two nodes are considered equal if they compare as equal.

Returns `  NULL  ` if `  graph_path  ` is `  NULL  ` .

**Return type**

`  BOOL  `

**Examples**

``` text
GRAPH FinGraph
MATCH p=(src:Account)-[t1:Transfers]->(mid:Account)-[t2:Transfers]->(dst:Account)
RETURN src.id AS source_account_id, IS_ACYCLIC(p) AS is_acyclic_path

/*-------------------------------------+
 | source_account_id | is_acyclic_path |
 +-------------------------------------+
 | 16                | TRUE            |
 | 20                | TRUE            |
 | 20                | TRUE            |
 | 16                | FALSE           |
 | 7                 | TRUE            |
 | 7                 | TRUE            |
 | 20                | FALSE           |
 +-------------------------------------*/
```

## `     IS_FIRST    `

``` text
IS_FIRST(k)
OVER over_clause

over_clause:
  ( [ window_specification ] )

window_specification:
  [ PARTITION BY partition_expression [, ...] ]
  [ ORDER BY expression [ { ASC | DESC }  ] [, ...] ]
```

**Description**

Returns `  true  ` if the current row is in the first `  k  ` rows (1-based) in the window; otherwise, returns `  false  ` . This function doesn't require the `  ORDER BY  ` clause.

**Details**

  - The `  k  ` value must be positive; otherwise, a runtime error is raised.
  - If any rows are tied or if `  ORDER BY  ` is omitted, the result is non-deterministic. If the `  ORDER BY  ` clause is unspecified or if all rows are tied, the result is equivalent to `  ANY-k  ` .

**Return Type**

`  BOOL  `

## `     IS_SIMPLE    `

``` text
IS_SIMPLE(graph_path)
```

**Description**

Checks if a graph path is simple. Returns `  TRUE  ` if the path has no repeated nodes, or if the only repeated nodes are its head and tail. Otherwise, returns `  FALSE  ` .

**Definitions**

  - `  graph_path  ` : A `  GRAPH_PATH  ` value that represents a graph path.

**Details**

Returns `  NULL  ` if `  graph_path  ` is `  NULL  ` .

**Return type**

`  BOOL  `

**Examples**

``` text
GRAPH FinGraph
MATCH p=(a1:Account)-[t1:Transfers where t1.amount > 200]->
        (a2:Account)-[t2:Transfers where t2.amount > 200]->
        (a3:Account)-[t3:Transfers where t3.amount > 100]->(a4:Account)
RETURN
  IS_SIMPLE(p) AS is_simple_path,
  a1.id as a1_id, a2.id as a2_id, a3.id as a3_id, a4.id as a4_id

/*----------------+-------+-------+-------+-------+
 | is_simple_path | a1_id | a2_id | a3_id | a4_id |
 +----------------+-------+-------+-------+-------+
 | TRUE           | 7     | 16    | 20    | 7     |
 | TRUE           | 16    | 20    | 7     | 16    |
 | FALSE          | 7     | 16    | 20    | 16    |
 | TRUE           | 20    | 7     | 16    | 20    |
 +----------------+-------+-------+-------+-------*/
```

## `     IS_TRAIL    `

``` text
IS_TRAIL(graph_path)
```

**Description**

Checks if a graph path has a repeating edge. Returns `  TRUE  ` if a repetition isn't found, otherwise returns `  FALSE  ` .

**Definitions**

  - `  graph_path  ` : A `  GRAPH_PATH  ` value that represents a graph path.

**Details**

Returns `  NULL  ` if `  graph_path  ` is `  NULL  ` .

**Return type**

`  BOOL  `

**Examples**

``` text
GRAPH FinGraph
MATCH
  p=(a1:Account)-[t1:Transfers]->(a2:Account)-[t2:Transfers]->
    (a3:Account)-[t3:Transfers]->(a4:Account)
WHERE a1.id < a4.id
RETURN
  IS_TRAIL(p) AS is_trail_path, t1.id as t1_id, t2.id as t2_id, t3.id as t3_id

/*---------------+-------+-------+-------+
 | is_trail_path | t1_id | t2_id | t3_id |
 +---------------+-------+-------+-------+
 | FALSE         | 16    | 20    | 16    |
 | TRUE          | 7     | 16    | 20    |
 | TRUE          | 7     | 16    | 20    |
 +---------------+-------+-------+-------*/
```

## `     LABELS    `

``` text
LABELS(element)
```

**Description**

Gets the labels associated with a graph element and preserves the original case of each label.

**Arguments**

  - `  element  ` : A `  GRAPH_ELEMENT  ` value that represents the graph element to extract labels from.

**Details**

Returns `  NULL  ` if `  element  ` is `  NULL  ` .

**Note:** Labels in a graph element are uniquely identified by their names. Labels are case insensitive. A defined label in the schema always takes precedence over [dynamic label](/spanner/docs/reference/standard-sql/graph-schema-statements#dynamic_label_definition) when their names conflict. To learn how to model dynamic labels, see the [dynamic label definition](/spanner/docs/reference/standard-sql/graph-schema-statements#dynamic_label_definition) .

**Return type**

`  ARRAY<STRING>  `

**Examples**

``` text
GRAPH FinGraph
MATCH (n:Person|Account)
RETURN LABELS(n) AS label, n.id

/*----------------+
 | label     | id |
 +----------------+
 | [Account] | 7  |
 | [Account] | 16 |
 | [Account] | 20 |
 | [Person]  | 1  |
 | [Person]  | 2  |
 | [Person]  | 3  |
 +----------------*/
```

## `     NODES    `

``` text
NODES(graph_path)
```

**Description**

Gets the nodes in a graph path. The resulting array retains the original order in the graph path.

**Definitions**

  - `  graph_path  ` : A `  GRAPH_PATH  ` value that represents a graph path.

**Details**

Returns `  NULL  ` if `  graph_path  ` is `  NULL  ` .

**Return type**

`  ARRAY<GRAPH_ELEMENT>  `

**Examples**

``` text
GRAPH FinGraph
MATCH p=(src:Account)-[t1:Transfers]->(mid:Account)-[t2:Transfers]->(dst:Account)
LET ns = NODES(p)
RETURN
  JSON_QUERY(TO_JSON(ns)[0], '$.labels') AS labels,
  JSON_QUERY(TO_JSON(ns)[0], '$.properties.nick_name') AS nick_name;

/*--------------------------------+
 | labels      | nick_name        |
 +--------------------------------+
 | ["Account"] | "Vacation Fund"  |
 | ["Account"] | "Rainy Day Fund" |
 | ["Account"] | "Rainy Day Fund" |
 | ["Account"] | "Rainy Day Fund" |
 | ["Account"] | "Vacation Fund"  |
 | ["Account"] | "Vacation Fund"  |
 | ["Account"] | "Vacation Fund"  |
 | ["Account"] | "Rainy Day Fund" |
 +--------------------------------*/
```

## `     PATH    `

``` text
PATH(graph_element[, ...])
```

**Description**

Creates a graph path from a list of graph elements.

**Definitions**

  - `  graph_element  ` : A `  GRAPH_ELEMENT  ` value that represents a graph element, such as a node or edge, to add to a graph path.

**Details**

This function produces an error if:

  - A graph element is `  NULL  ` .
  - Nodes aren't interleaved with edges.
  - An edge doesn't connect to neighboring nodes.

**Return type**

`  GRAPH_PATH  `

**Examples**

``` text
GRAPH FinGraph
MATCH (src:Account)-[t1:Transfers]->(mid:Account)-[t2:Transfers]->(dst:Account)
LET p = PATH(src, t1, mid, t2, dst)
RETURN
  JSON_QUERY(TO_JSON(p)[0], '$.labels') AS element_a,
  JSON_QUERY(TO_JSON(p)[1], '$.labels') AS element_b,
  JSON_QUERY(TO_JSON(p)[2], '$.labels') AS element_c

/*-------------------------------------------+
 | element_a   | element_b     | element_c   |
 +-------------------------------------------+
 | ["Account"] | ["Transfers"] | ["Account"] |
 | ...         | ...           | ...         |
 +-------------------------------------------*/
```

``` text
-- Error: in 'p', a graph element is NULL.
GRAPH FinGraph
MATCH (src:Account)-[t1:Transfers]->(mid:Account)-[t2:Transfers]->(dst:Account)
LET p = PATH(src, NULL, mid, t2, dst)
RETURN TO_JSON(p) AS results
```

``` text
-- Error: in 'p', 'src' and 'mid' are nodes that should be interleaved with an
-- edge.
GRAPH FinGraph
MATCH (src:Account)-[t1:Transfers]->(mid:Account)-[t2:Transfers]->(dst:Account)
LET p = PATH(src, mid, t2, dst)
RETURN TO_JSON(p) AS results
```

``` text
-- Error: in 'p', 't2' is an edge that doesn't connect to a neighboring node on
-- the right.
GRAPH FinGraph
MATCH (src:Account)-[t1:Transfers]->(mid:Account)-[t2:Transfers]->(dst:Account)
LET p = PATH(src, t2, mid)
RETURN TO_JSON(p) AS results
```

## `     PATH_FIRST    `

``` text
PATH_FIRST(graph_path)
```

**Description**

Gets the first node in a graph path.

**Definitions**

  - `  graph_path  ` : A `  GRAPH_PATH  ` value that represents the graph path to extract the first node from.

**Details**

Returns `  NULL  ` if `  graph_path  ` is `  NULL  ` .

**Return type**

`  GRAPH_ELEMENT  `

**Examples**

``` text
GRAPH FinGraph
MATCH p=(src:Account)-[t1:Transfers]->(mid:Account)-[t2:Transfers]->(dst:Account)
LET f = PATH_FIRST(p)
RETURN
  LABELS(f) AS labels,
  f.nick_name AS nick_name;

/*--------------------------+
 | labels  | nick_name      |
 +--------------------------+
 | Account | Vacation Fund  |
 | Account | Rainy Day Fund |
 | Account | Rainy Day Fund |
 | Account | Vacation Fund  |
 | Account | Vacation Fund  |
 | Account | Vacation Fund  |
 | Account | Rainy Day Fund |
 +--------------------------*/
```

## `     PATH_LAST    `

``` text
PATH_LAST(graph_path)
```

**Description**

Gets the last node in a graph path.

**Definitions**

  - `  graph_path  ` : A `  GRAPH_PATH  ` value that represents the graph path to extract the last node from.

**Details**

Returns `  NULL  ` if `  graph_path  ` is `  NULL  ` .

**Return type**

`  GRAPH_ELEMENT  `

**Examples**

``` text
GRAPH FinGraph
MATCH p=(src:Account)-[t1:Transfers]->(mid:Account)-[t2:Transfers]->(dst:Account)
LET f = PATH_LAST(p)
RETURN
  LABELS(f) AS labels,
  f.nick_name AS nick_name;

/*--------------------------+
 | labels  | nick_name      |
 +--------------------------+
 | Account | Vacation Fund  |
 | Account | Vacation Fund  |
 | Account | Vacation Fund  |
 | Account | Vacation Fund  |
 | Account | Rainy Day Fund |
 | Account | Rainy Day Fund |
 | Account | Rainy Day Fund |
 +--------------------------*/
```

## `     PATH_LENGTH    `

``` text
PATH_LENGTH(graph_path)
```

**Description**

Gets the number of edges in a graph path.

**Definitions**

  - `  graph_path  ` : A `  GRAPH_PATH  ` value that represents the graph path with the edges to count.

**Details**

Returns `  NULL  ` if `  graph_path  ` is `  NULL  ` .

**Return type**

`  INT64  `

**Examples**

``` text
GRAPH FinGraph
MATCH p=(src:Account)-[t1:Transfers]->(mid:Account)-[t2:Transfers]->(dst:Account)
RETURN PATH_LENGTH(p) AS results

/*---------+
 | results |
 +---------+
 | 2       |
 | 2       |
 | 2       |
 | 2       |
 | 2       |
 | 2       |
 | 2       |
 +---------*/
```

## `     PROPERTY_NAMES    `

``` text
PROPERTY_NAMES(element)
```

**Description**

Gets the name of each property associated with a graph element and preserves the original case of each name.

**Note:** Properties in a graph element are uniquely identified by their names. Properties are case insensitive. Defined properties in the schema always take precedence over [dynamic properties](/spanner/docs/reference/standard-sql/graph-schema-statements#dynamic_label_definition) when their names conflict. For more information, see [dynamic properties definition](/spanner/docs/reference/standard-sql/graph-schema-statements#dynamic_properties_definition) .

**Arguments**

  - `  element  ` : A `  GRAPH_ELEMENT  ` value.

**Details**

Returns `  NULL  ` if `  element  ` is `  NULL  ` .

**Return type**

`  ARRAY<STRING>  `

**Examples**

``` text
GRAPH FinGraph
MATCH (n:Person|Account)
RETURN PROPERTY_NAMES(n) AS property_names, n.id

/*-----------------------------------------------+
 | label                                    | id |
 +-----------------------------------------------+
 | [create_time, id, is_blocked, nick_name] | 7  |
 | [create_time, id, is_blocked, nick_name] | 16 |
 | [create_time, id, is_blocked, nick_name] | 20 |
 | [birthday, city, country, id, name]      | 1  |
 | [birthday, city, country, id, name]      | 2  |
 | [birthday, city, country, id, name]      | 3  |
 +-----------------------------------------------*/
```

## `     SOURCE_NODE_ID    `

``` text
SOURCE_NODE_ID(edge_element)
```

**Description**

Gets a unique identifier of a graph edge's source node. The unique identifier is only valid for the scope of the query where it's obtained.

**Arguments**

  - `  edge_element  ` : A `  GRAPH_ELEMENT  ` value that represents an edge.

**Details**

Returns `  NULL  ` if `  edge_element  ` is `  NULL  ` .

**Return type**

`  STRING  `

**Examples**

``` text
GRAPH FinGraph
MATCH (p:Person)-[o:Owns]->(:Account)
RETURN p.name AS name, SOURCE_NODE_ID(o) AS source_node_id

/*-------------------------------------+
 | name | source_node_id               |
 +------|------------------------------+
 | Alex | mUZpbkdyYXBoLlBlcnNvbgB4kQI= |
 | Dana | mUZpbkdyYXBoLlBlcnNvbgB4kQQ= |
 | Lee  | mUZpbkdyYXBoLlBlcnNvbgB4kQY= |
 +-------------------------------------*/
```

Note that the actual identifiers obtained may be different from what's shown above.
