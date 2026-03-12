Graph Query Language (GQL) is a language designed to query graph data. This page describes the high level structure of GQL.

## Statement and clause

In GQL, a statement refers to a complete unit of execution, and a clause represents a modifier to statements. See the [statement list](/spanner/docs/reference/standard-sql/graph-query-statements#language_list) for a complete list.

## Working table

A working table refers to the intermediate table representing the input or output of a GQL statement.

A GQL statement receives an incoming working table and produces an outgoing working table.

The first incoming working table is a table with a single row. The last outgoing working table is returned as the query results.

## Linear query statement

A linear query statement consists of multiple statements from the [statement list](/spanner/docs/reference/standard-sql/graph-query-statements#language_list) . It always ends with a [`  RETURN  ` statement](/spanner/docs/reference/standard-sql/graph-query-statements#gql_return) .

Each statement generates intermediate results (the working table) and then passes those results to the next statement. The output of a linear query statement comes from the final `  RETURN  ` statement.

#### Examples

``` text
GRAPH FinGraph
MATCH (p:Person)-[o:Owns]->(a:Account)
FILTER p.birthday < '1990-01-10'
RETURN p.name

/*------+
 | name |
 +------+
 | Dana |
 | Lee  |
 +------*/
```

## Combining linear query statements with set operators

You can use a set operator to combine multiple linear query statements into one. For more information, see the syntax for the [GQL set operation](/spanner/docs/reference/standard-sql/graph-query-statements#gql_set) .

#### Examples

A set operator between two linear query statements with the same set of output column names and types but with different column orders is supported. For example:

``` text
GRAPH FinGraph
MATCH (p:Person)
RETURN p.name, 1 AS group_id
UNION ALL
MATCH (p:Person)
RETURN 2 AS group_id, p.name

/*------+----------+
 | name | group_id |
 +------+----------+
 | Alex |    1     |
 | Dana |    1     |
 | Lee  |    1     |
 | Alex |    2     |
 | Dana |    2     |
 | Lee  |    2     |
 +------+----------*/
```

## Chaining linear query statements with the `     NEXT    ` statement

You can use the `  NEXT  ` keyword to chain multiple linear query statements into one.

The final linear query statement must produce non-GQL data types, but linear query statements that proceed it can produce GQL data types, such as nodes and edges.

#### Examples

The following is an example of a graph query chaining multiple linear query statements using `  NEXT  ` .

``` text
GRAPH FinGraph

MATCH (a:Account {is_blocked: TRUE})
RETURN a
UNION ALL
MATCH (a:Account)<-[:Owns]-(p:Person {id: 2})
RETURN a

NEXT

MATCH (a:Account)-[t:Transfers]->(oa:Account)
WITH DISTINCT oa
RETURN oa.nick_name

/*----------------+
 | nick_name      |
 +----------------+
 | Vacation Fund  |
 | Vacation Fund  |
 | Rainy Day Fund |
 +----------------*/
```
