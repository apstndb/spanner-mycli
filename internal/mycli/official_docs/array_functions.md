GoogleSQL for Spanner supports the following array functions.

## Function list

<table>
<colgroup>
<col style="width: 50%" />
<col style="width: 50%" />
</colgroup>
<thead>
<tr class="header">
<th>Name</th>
<th>Summary</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array"><code dir="ltr" translate="no">        ARRAY       </code></a></td>
<td>Produces an array with one element for each row in a subquery.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#array_agg"><code dir="ltr" translate="no">        ARRAY_AGG       </code></a></td>
<td>Gets an array of values.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/aggregate_functions">Aggregate functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_concat"><code dir="ltr" translate="no">        ARRAY_CONCAT       </code></a></td>
<td>Concatenates one or more arrays with the same element type into a single array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#array_concat_agg"><code dir="ltr" translate="no">        ARRAY_CONCAT_AGG       </code></a></td>
<td>Concatenates arrays and returns a single array as a result.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/aggregate_functions">Aggregate functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_filter"><code dir="ltr" translate="no">        ARRAY_FILTER       </code></a></td>
<td>Takes an array, filters out unwanted elements, and returns the results in a new array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_first"><code dir="ltr" translate="no">        ARRAY_FIRST       </code></a></td>
<td>Gets the first element in an array.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_includes"><code dir="ltr" translate="no">        ARRAY_INCLUDES       </code></a></td>
<td>Checks if there is an element in the array that is equal to a search value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_includes_all"><code dir="ltr" translate="no">        ARRAY_INCLUDES_ALL       </code></a></td>
<td>Checks if all search values are in an array.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_includes_any"><code dir="ltr" translate="no">        ARRAY_INCLUDES_ANY       </code></a></td>
<td>Checks if any search values are in an array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_is_distinct"><code dir="ltr" translate="no">        ARRAY_IS_DISTINCT       </code></a></td>
<td>Checks if an array contains no repeated elements.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_last"><code dir="ltr" translate="no">        ARRAY_LAST       </code></a></td>
<td>Gets the last element in an array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_length"><code dir="ltr" translate="no">        ARRAY_LENGTH       </code></a></td>
<td>Gets the number of elements in an array.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_max"><code dir="ltr" translate="no">        ARRAY_MAX       </code></a></td>
<td>Gets the maximum non- <code dir="ltr" translate="no">       NULL      </code> value in an array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_min"><code dir="ltr" translate="no">        ARRAY_MIN       </code></a></td>
<td>Gets the minimum non- <code dir="ltr" translate="no">       NULL      </code> value in an array.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_reverse"><code dir="ltr" translate="no">        ARRAY_REVERSE       </code></a></td>
<td>Reverses the order of elements in an array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_slice"><code dir="ltr" translate="no">        ARRAY_SLICE       </code></a></td>
<td>Produces an array containing zero or more consecutive elements from an input array.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_to_string"><code dir="ltr" translate="no">        ARRAY_TO_STRING       </code></a></td>
<td>Produces a concatenation of the elements in an array as a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_transform"><code dir="ltr" translate="no">        ARRAY_TRANSFORM       </code></a></td>
<td>Transforms the elements of an array, and returns the results in a new array.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#generate_array"><code dir="ltr" translate="no">        GENERATE_ARRAY       </code></a></td>
<td>Generates an array of values in a range.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#generate_date_array"><code dir="ltr" translate="no">        GENERATE_DATE_ARRAY       </code></a></td>
<td>Generates an array of dates in a range.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_array"><code dir="ltr" translate="no">        JSON_ARRAY       </code></a></td>
<td>Creates a JSON array.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_array_append"><code dir="ltr" translate="no">        JSON_ARRAY_APPEND       </code></a></td>
<td>Appends JSON data to the end of a JSON array.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_array_insert"><code dir="ltr" translate="no">        JSON_ARRAY_INSERT       </code></a></td>
<td>Inserts JSON data into a JSON array.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_query_array"><code dir="ltr" translate="no">        JSON_QUERY_ARRAY       </code></a></td>
<td>Extracts a JSON array and converts it to a SQL <code dir="ltr" translate="no">       ARRAY&lt;JSON-formatted STRING&gt;      </code> or <code dir="ltr" translate="no">       ARRAY&lt;JSON&gt;      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_value_array"><code dir="ltr" translate="no">        JSON_VALUE_ARRAY       </code></a></td>
<td>Extracts a JSON array of scalar values and converts it to a SQL <code dir="ltr" translate="no">       ARRAY&lt;STRING&gt;      </code> value.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/json_functions">JSON functions</a> .</td>
</tr>
</tbody>
</table>

## `     ARRAY    `

``` text
ARRAY(subquery)
```

**Description**

The `  ARRAY  ` function returns an `  ARRAY  ` with one element for each row in a [subquery](/spanner/docs/reference/standard-sql/subqueries) .

If `  subquery  ` produces a SQL table, the table must have exactly one column. Each element in the output `  ARRAY  ` is the value of the single column of a row in the table.

If `  subquery  ` produces a value table, then each element in the output `  ARRAY  ` is the entire corresponding row of the value table.

**Constraints**

  - Subqueries are unordered, so the elements of the output `  ARRAY  ` aren't guaranteed to preserve any order in the source table for the subquery. However, if the subquery includes an `  ORDER BY  ` clause, the `  ARRAY  ` function will return an `  ARRAY  ` that honors that clause.
  - If the subquery returns more than one column, the `  ARRAY  ` function returns an error.
  - If the subquery returns an `  ARRAY  ` typed column or `  ARRAY  ` typed rows, the `  ARRAY  ` function returns an error that GoogleSQL doesn't support `  ARRAY  ` s with elements of type [`  ARRAY  `](/spanner/docs/reference/standard-sql/data-types#array_type) .
  - If the subquery returns zero rows, the `  ARRAY  ` function returns an empty `  ARRAY  ` . It never returns a `  NULL  ` `  ARRAY  ` .

**Return type**

`  ARRAY  `

**Examples**

``` text
SELECT ARRAY
  (SELECT 1 UNION ALL
   SELECT 2 UNION ALL
   SELECT 3) AS new_array;

/*-----------+
 | new_array |
 +-----------+
 | [1, 2, 3] |
 +-----------*/
```

To construct an `  ARRAY  ` from a subquery that contains multiple columns, change the subquery to use `  SELECT AS STRUCT  ` . Now the `  ARRAY  ` function will return an `  ARRAY  ` of `  STRUCT  ` s. The `  ARRAY  ` will contain one `  STRUCT  ` for each row in the subquery, and each of these `  STRUCT  ` s will contain a field for each column in that row.

``` text
SELECT
  ARRAY
    (SELECT AS STRUCT 1, 2, 3
     UNION ALL SELECT AS STRUCT 4, 5, 6) AS new_array;

/*------------------------+
 | new_array              |
 +------------------------+
 | [{1, 2, 3}, {4, 5, 6}] |
 +------------------------*/
```

Similarly, to construct an `  ARRAY  ` from a subquery that contains one or more `  ARRAY  ` s, change the subquery to use `  SELECT AS STRUCT  ` .

``` text
SELECT ARRAY
  (SELECT AS STRUCT [1, 2, 3] UNION ALL
   SELECT AS STRUCT [4, 5, 6]) AS new_array;

/*----------------------------+
 | new_array                  |
 +----------------------------+
 | [{[1, 2, 3]}, {[4, 5, 6]}] |
 +----------------------------*/
```

## `     ARRAY_CONCAT    `

``` text
ARRAY_CONCAT(array_expression[, ...])
```

**Description**

Concatenates one or more arrays with the same element type into a single array.

The function returns `  NULL  ` if any input argument is `  NULL  ` .

**Note:** You can also use the [|| concatenation operator](/spanner/docs/reference/standard-sql/operators) to concatenate arrays.

**Return type**

`  ARRAY  `

**Examples**

``` text
SELECT ARRAY_CONCAT([1, 2], [3, 4], [5, 6]) as count_to_six;

/*--------------------------------------------------+
 | count_to_six                                     |
 +--------------------------------------------------+
 | [1, 2, 3, 4, 5, 6]                               |
 +--------------------------------------------------*/
```

## `     ARRAY_FILTER    `

``` text
ARRAY_FILTER(array_expression, lambda_expression)

lambda_expression:
  {
    element_alias -> boolean_expression
    | (element_alias, index_alias) -> boolean_expression
  }
```

**Description**

Takes an array, filters out unwanted elements, and returns the results in a new array.

  - `  array_expression  ` : The array to filter.
  - `  lambda_expression  ` : Each element in `  array_expression  ` is evaluated against the [lambda expression](/spanner/docs/reference/standard-sql/functions-reference#lambdas) . If the expression evaluates to `  FALSE  ` or `  NULL  ` , the element is removed from the resulting array.
  - `  element_alias  ` : An alias that represents an array element.
  - `  index_alias  ` : An alias that represents the zero-based offset of the array element.
  - `  boolean_expression  ` : The predicate used to filter the array elements.

Returns `  NULL  ` if the `  array_expression  ` is `  NULL  ` .

**Return type**

ARRAY

**Example**

``` text
SELECT
  ARRAY_FILTER([1 ,2, 3], e -> e > 1) AS a1,
  ARRAY_FILTER([0, 2, 3], (e, i) -> e > i) AS a2;

/*-------+-------+
 | a1    | a2    |
 +-------+-------+
 | [2,3] | [2,3] |
 +-------+-------*/
```

## `     ARRAY_FIRST    `

``` text
ARRAY_FIRST(array_expression)
```

**Description**

Takes an array and returns the first element in the array.

Produces an error if the array is empty.

Returns `  NULL  ` if `  array_expression  ` is `  NULL  ` .

**Note:** To get the last element in an array, see [`  ARRAY_LAST  `](#array_last) .

**Return type**

Matches the data type of elements in `  array_expression  ` .

**Example**

``` text
SELECT ARRAY_FIRST(['a','b','c','d']) as first_element

/*---------------+
 | first_element |
 +---------------+
 | a             |
 +---------------*/
```

## `     ARRAY_INCLUDES    `

  - [Signature 1](#array_includes_signature1) : `  ARRAY_INCLUDES(array_to_search, search_value)  `
  - [Signature 2](#array_includes_signature2) : `  ARRAY_INCLUDES(array_to_search, lambda_expression)  `

#### Signature 1

``` text
ARRAY_INCLUDES(array_to_search, search_value)
```

**Description**

Takes an array and returns `  TRUE  ` if there is an element in the array that is equal to the search\_value.

  - `  array_to_search  ` : The array to search.
  - `  search_value  ` : The element to search for in the array.

Returns `  NULL  ` if `  array_to_search  ` or `  search_value  ` is `  NULL  ` .

**Return type**

`  BOOL  `

**Example**

In the following example, the query first checks to see if `  0  ` exists in an array. Then the query checks to see if `  1  ` exists in an array.

``` text
SELECT
  ARRAY_INCLUDES([1, 2, 3], 0) AS a1,
  ARRAY_INCLUDES([1, 2, 3], 1) AS a2;

/*-------+------+
 | a1    | a2   |
 +-------+------+
 | false | true |
 +-------+------*/
```

#### Signature 2

``` text
ARRAY_INCLUDES(array_to_search, lambda_expression)

lambda_expression: element_alias -> boolean_expression
```

**Description**

Takes an array and returns `  TRUE  ` if the lambda expression evaluates to `  TRUE  ` for any element in the array.

  - `  array_to_search  ` : The array to search.
  - `  lambda_expression  ` : Each element in `  array_to_search  ` is evaluated against the [lambda expression](/spanner/docs/reference/standard-sql/functions-reference#lambdas) .
  - `  element_alias  ` : An alias that represents an array element.
  - `  boolean_expression  ` : The predicate used to evaluate the array elements.

Returns `  NULL  ` if `  array_to_search  ` is `  NULL  ` .

**Return type**

`  BOOL  `

**Example**

In the following example, the query first checks to see if any elements that are greater than 3 exist in an array ( `  e > 3  ` ). Then the query checks to see if any elements that are greater than 0 exist in an array ( `  e > 0  ` ).

``` text
SELECT
  ARRAY_INCLUDES([1, 2, 3], e -> e > 3) AS a1,
  ARRAY_INCLUDES([1, 2, 3], e -> e > 0) AS a2;

/*-------+------+
 | a1    | a2   |
 +-------+------+
 | false | true |
 +-------+------*/
```

## `     ARRAY_INCLUDES_ALL    `

``` text
ARRAY_INCLUDES_ALL(array_to_search, search_values)
```

**Description**

Takes an array to search and an array of search values. Returns `  TRUE  ` if all search values are in the array to search, otherwise returns `  FALSE  ` .

  - `  array_to_search  ` : The array to search.
  - `  search_values  ` : The array that contains the elements to search for.

Returns `  NULL  ` if `  array_to_search  ` or `  search_values  ` is `  NULL  ` .

**Return type**

`  BOOL  `

**Example**

In the following example, the query first checks to see if `  3  ` , `  4  ` , and `  5  ` exists in an array. Then the query checks to see if `  4  ` , `  5  ` , and `  6  ` exists in an array.

``` text
SELECT
  ARRAY_INCLUDES_ALL([1,2,3,4,5], [3,4,5]) AS a1,
  ARRAY_INCLUDES_ALL([1,2,3,4,5], [4,5,6]) AS a2;

/*------+-------+
 | a1   | a2    |
 +------+-------+
 | true | false |
 +------+-------*/
```

## `     ARRAY_INCLUDES_ANY    `

``` text
ARRAY_INCLUDES_ANY(array_to_search, search_values)
```

**Description**

Takes an array to search and an array of search values. Returns `  TRUE  ` if any search values are in the array to search, otherwise returns `  FALSE  ` .

  - `  array_to_search  ` : The array to search.
  - `  search_values  ` : The array that contains the elements to search for.

Returns `  NULL  ` if `  array_to_search  ` or `  search_values  ` is `  NULL  ` .

**Return type**

`  BOOL  `

**Example**

In the following example, the query first checks to see if `  3  ` , `  4  ` , or `  5  ` exists in an array. Then the query checks to see if `  4  ` , `  5  ` , or `  6  ` exists in an array.

``` text
SELECT
  ARRAY_INCLUDES_ANY([1,2,3], [3,4,5]) AS a1,
  ARRAY_INCLUDES_ANY([1,2,3], [4,5,6]) AS a2;

/*------+-------+
 | a1   | a2    |
 +------+-------+
 | true | false |
 +------+-------*/
```

## `     ARRAY_IS_DISTINCT    `

``` text
ARRAY_IS_DISTINCT(value)
```

**Description**

Returns `  TRUE  ` if the array contains no repeated elements, using the same equality comparison logic as `  SELECT DISTINCT  ` .

**Return type**

`  BOOL  `

**Examples**

``` text
SELECT ARRAY_IS_DISTINCT([1, 2, 3]) AS is_distinct

/*-------------+
 | is_distinct |
 +-------------+
 | true        |
 +-------------*/
```

``` text
SELECT ARRAY_IS_DISTINCT([1, 1, 1]) AS is_distinct

/*-------------+
 | is_distinct |
 +-------------+
 | false       |
 +-------------*/
```

``` text
SELECT ARRAY_IS_DISTINCT([1, 2, NULL]) AS is_distinct

/*-------------+
 | is_distinct |
 +-------------+
 | true        |
 +-------------*/
```

``` text
SELECT ARRAY_IS_DISTINCT([1, 1, NULL]) AS is_distinct

/*-------------+
 | is_distinct |
 +-------------+
 | false       |
 +-------------*/
```

``` text
SELECT ARRAY_IS_DISTINCT([1, NULL, NULL]) AS is_distinct

/*-------------+
 | is_distinct |
 +-------------+
 | false       |
 +-------------*/
```

``` text
SELECT ARRAY_IS_DISTINCT([]) AS is_distinct

/*-------------+
 | is_distinct |
 +-------------+
 | true        |
 +-------------*/
```

``` text
SELECT ARRAY_IS_DISTINCT(NULL) AS is_distinct

/*-------------+
 | is_distinct |
 +-------------+
 | NULL        |
 +-------------*/
```

## `     ARRAY_LAST    `

``` text
ARRAY_LAST(array_expression)
```

**Description**

Takes an array and returns the last element in the array.

Produces an error if the array is empty.

Returns `  NULL  ` if `  array_expression  ` is `  NULL  ` .

**Note:** To get the first element in an array, see [`  ARRAY_FIRST  `](#array_first) .

**Return type**

Matches the data type of elements in `  array_expression  ` .

**Example**

``` text
SELECT ARRAY_LAST(['a','b','c','d']) as last_element

/*---------------+
 | last_element  |
 +---------------+
 | d             |
 +---------------*/
```

## `     ARRAY_LENGTH    `

``` text
ARRAY_LENGTH(array_expression)
```

**Description**

Returns the size of the array. Returns 0 for an empty array. Returns `  NULL  ` if the `  array_expression  ` is `  NULL  ` .

**Return type**

`  INT64  `

**Examples**

``` text
SELECT
  ARRAY_LENGTH(["coffee", NULL, "milk" ]) AS size_a,
  ARRAY_LENGTH(["cake", "pie"]) AS size_b;

/*--------+--------+
 | size_a | size_b |
 +--------+--------+
 | 3      | 2      |
 +--------+--------*/
```

## `     ARRAY_MAX    `

``` text
ARRAY_MAX(input_array)
```

**Description**

Returns the maximum non- `  NULL  ` value in an array.

Caveats:

  - If the array is `  NULL  ` , empty, or contains only `  NULL  ` s, returns `  NULL  ` .
  - If the array contains `  NaN  ` , returns `  NaN  ` .

**Supported Argument Types**

In the input array, `  ARRAY<T>  ` , `  T  ` can be an [orderable data type](/spanner/docs/reference/standard-sql/data-types#data_type_properties) .

**Return type**

The same data type as `  T  ` in the input array.

**Examples**

``` text
SELECT ARRAY_MAX([8, 37, NULL, 55, 4]) as max

/*-----+
 | max |
 +-----+
 | 55  |
 +-----*/
```

## `     ARRAY_MIN    `

``` text
ARRAY_MIN(input_array)
```

**Description**

Returns the minimum non- `  NULL  ` value in an array.

Caveats:

  - If the array is `  NULL  ` , empty, or contains only `  NULL  ` s, returns `  NULL  ` .
  - If the array contains `  NaN  ` , returns `  NaN  ` .

**Supported Argument Types**

In the input array, `  ARRAY<T>  ` , `  T  ` can be an [orderable data type](/spanner/docs/reference/standard-sql/data-types#data_type_properties) .

**Return type**

The same data type as `  T  ` in the input array.

**Examples**

``` text
SELECT ARRAY_MIN([8, 37, NULL, 4, 55]) as min

/*-----+
 | min |
 +-----+
 | 4   |
 +-----*/
```

## `     ARRAY_REVERSE    `

``` text
ARRAY_REVERSE(value)
```

**Description**

Returns the input `  ARRAY  ` with elements in reverse order.

**Return type**

`  ARRAY  `

**Examples**

``` text
SELECT ARRAY_REVERSE([1, 2, 3]) AS reverse_arr

/*-------------+
 | reverse_arr |
 +-------------+
 | [3, 2, 1]   |
 +-------------*/
```

## `     ARRAY_SLICE    `

``` text
ARRAY_SLICE(array_to_slice, start_offset, end_offset)
```

**Description**

Returns an array containing zero or more consecutive elements from the input array.

  - `  array_to_slice  ` : The array that contains the elements you want to slice.
  - `  start_offset  ` : The inclusive starting offset.
  - `  end_offset  ` : The inclusive ending offset.

An offset can be positive or negative. A positive offset starts from the beginning of the input array and is 0-based. A negative offset starts from the end of the input array. Out-of-bounds offsets are supported. Here are some examples:

<table>
<thead>
<tr class="header">
<th>Input offset</th>
<th>Final offset in array</th>
<th>Notes</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>0</td>
<td>[ <strong>'a'</strong> , 'b', 'c', 'd']</td>
<td>The final offset is <code dir="ltr" translate="no">       0      </code> .</td>
</tr>
<tr class="even">
<td>3</td>
<td>['a', 'b', 'c', <strong>'d'</strong> ]</td>
<td>The final offset is <code dir="ltr" translate="no">       3      </code> .</td>
</tr>
<tr class="odd">
<td>5</td>
<td>['a', 'b', 'c', <strong>'d'</strong> ]</td>
<td>Because the input offset is out of bounds, the final offset is <code dir="ltr" translate="no">       3      </code> ( <code dir="ltr" translate="no">       array length - 1      </code> ).</td>
</tr>
<tr class="even">
<td>-1</td>
<td>['a', 'b', 'c', <strong>'d'</strong> ]</td>
<td>Because a negative offset is used, the offset starts at the end of the array. The final offset is <code dir="ltr" translate="no">       3      </code> ( <code dir="ltr" translate="no">       array length - 1      </code> ).</td>
</tr>
<tr class="odd">
<td>-2</td>
<td>['a', 'b', <strong>'c'</strong> , 'd']</td>
<td>Because a negative offset is used, the offset starts at the end of the array. The final offset is <code dir="ltr" translate="no">       2      </code> ( <code dir="ltr" translate="no">       array length - 2      </code> ).</td>
</tr>
<tr class="even">
<td>-4</td>
<td>[ <strong>'a'</strong> , 'b', 'c', 'd']</td>
<td>Because a negative offset is used, the offset starts at the end of the array. The final offset is <code dir="ltr" translate="no">       0      </code> ( <code dir="ltr" translate="no">       array length - 4      </code> ).</td>
</tr>
<tr class="odd">
<td>-5</td>
<td>[ <strong>'a'</strong> , 'b', 'c', 'd']</td>
<td>Because the offset is negative and out of bounds, the final offset is <code dir="ltr" translate="no">       0      </code> ( <code dir="ltr" translate="no">       array length - array length      </code> ).</td>
</tr>
</tbody>
</table>

Additional details:

  - The input array can contain `  NULL  ` elements. `  NULL  ` elements are included in the resulting array.
  - Returns `  NULL  ` if `  array_to_slice  ` , `  start_offset  ` , or `  end_offset  ` is `  NULL  ` .
  - Returns an empty array if `  array_to_slice  ` is empty.
  - Returns an empty array if the position of the `  start_offset  ` in the array is after the position of the `  end_offset  ` .

**Return type**

`  ARRAY  `

**Examples**

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], 1, 3) AS result

/*-----------+
 | result    |
 +-----------+
 | [b, c, d] |
 +-----------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], -1, 3) AS result

/*-----------+
 | result    |
 +-----------+
 | []        |
 +-----------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], 1, -3) AS result

/*--------+
 | result |
 +--------+
 | [b, c] |
 +--------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], -1, -3) AS result

/*-----------+
 | result    |
 +-----------+
 | []        |
 +-----------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], -3, -1) AS result

/*-----------+
 | result    |
 +-----------+
 | [c, d, e] |
 +-----------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], 3, 3) AS result

/*--------+
 | result |
 +--------+
 | [d]    |
 +--------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], -3, -3) AS result

/*--------+
 | result |
 +--------+
 | [c]    |
 +--------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], 1, 30) AS result

/*--------------+
 | result       |
 +--------------+
 | [b, c, d, e] |
 +--------------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], 1, -30) AS result

/*-----------+
 | result    |
 +-----------+
 | []        |
 +-----------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], -30, 30) AS result

/*-----------------+
 | result          |
 +-----------------+
 | [a, b, c, d, e] |
 +-----------------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], -30, -5) AS result

/*--------+
 | result |
 +--------+
 | [a]    |
 +--------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], 5, 30) AS result

/*--------+
 | result |
 +--------+
 | []     |
 +--------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', 'c', 'd', 'e'], 1, NULL) AS result

/*-----------+
 | result    |
 +-----------+
 | NULL      |
 +-----------*/
```

``` text
SELECT ARRAY_SLICE(['a', 'b', NULL, 'd', 'e'], 1, 3) AS result

/*--------------+
 | result       |
 +--------------+
 | [b, NULL, d] |
 +--------------*/
```

## `     ARRAY_TO_STRING    `

``` text
ARRAY_TO_STRING(array_expression, delimiter[, null_text])
```

**Description**

Returns a concatenation of the elements in `  array_expression  ` as a `  STRING  ` or `  BYTES  ` value. The value for `  array_expression  ` can either be an array of `  STRING  ` or `  BYTES  ` data type.

If the `  null_text  ` parameter is used, the function replaces any `  NULL  ` values in the array with the value of `  null_text  ` .

If the `  null_text  ` parameter isn't used, the function omits the `  NULL  ` value and its preceding delimiter.

**Return type**

  - `  STRING  ` for a function signature with `  STRING  ` input.
  - `  BYTES  ` for a function signature with `  BYTES  ` input.

**Examples**

``` text
SELECT ARRAY_TO_STRING(['coffee', 'tea', 'milk', NULL], '--', 'MISSING') AS text

/*--------------------------------+
 | text                           |
 +--------------------------------+
 | coffee--tea--milk--MISSING     |
 +--------------------------------*/
```

``` text
SELECT ARRAY_TO_STRING(['cake', 'pie', NULL], '--', 'MISSING') AS text

/*--------------------------------+
 | text                           |
 +--------------------------------+
 | cake--pie--MISSING             |
 +--------------------------------*/
```

``` text
SELECT ARRAY_TO_STRING([b'prefix', b'middle', b'suffix', b'\x00'], b'--') AS data

/*--------------------------------+
 | data                           |
 +--------------------------------+
 | prefix--middle--suffix--\x00   |
 +--------------------------------*/
```

## `     ARRAY_TRANSFORM    `

``` text
ARRAY_TRANSFORM(array_expression, lambda_expression)

lambda_expression:
  {
    element_alias -> transform_expression
    | (element_alias, index_alias) -> transform_expression
  }
```

**Description**

Takes an array, transforms the elements, and returns the results in a new array. The output array always has the same length as the input array.

  - `  array_expression  ` : The array to transform.
  - `  lambda_expression  ` : Each element in `  array_expression  ` is evaluated against the [lambda expression](/spanner/docs/reference/standard-sql/functions-reference#lambdas) . The evaluation results are returned in a new array.
  - `  element_alias  ` : An alias that represents an array element.
  - `  index_alias  ` : An alias that represents the zero-based offset of the array element.
  - `  transform_expression  ` : The expression used to transform the array elements.

Returns `  NULL  ` if the `  array_expression  ` is `  NULL  ` .

**Return type**

`  ARRAY  `

**Example**

``` text
SELECT
  ARRAY_TRANSFORM([1, 4, 3], e -> e + 1) AS a1,
  ARRAY_TRANSFORM([1, 4, 3], (e, i) -> e + i) AS a2;

/*---------+---------+
 | a1      | a2      |
 +---------+---------+
 | [2,5,4] | [1,5,5] |
 +---------+---------*/
```

## `     GENERATE_ARRAY    `

``` text
GENERATE_ARRAY(start_expression, end_expression[, step_expression])
```

**Description**

Returns an array of values. The `  start_expression  ` and `  end_expression  ` parameters determine the inclusive start and end of the array.

The `  GENERATE_ARRAY  ` function accepts the following data types as inputs:

  - `  INT64  `
  - `  NUMERIC  `
  - `  FLOAT64  `

The `  step_expression  ` parameter determines the increment used to generate array values. The default value for this parameter is `  1  ` .

This function returns an error if `  step_expression  ` is set to 0, or if any input is `  NaN  ` .

If any argument is `  NULL  ` , the function will return a `  NULL  ` array.

**Return Data Type**

`  ARRAY  `

**Examples**

The following returns an array of integers, with a default step of 1.

``` text
SELECT GENERATE_ARRAY(1, 5) AS example_array;

/*-----------------+
 | example_array   |
 +-----------------+
 | [1, 2, 3, 4, 5] |
 +-----------------*/
```

The following returns an array using a user-specified step size.

``` text
SELECT GENERATE_ARRAY(0, 10, 3) AS example_array;

/*---------------+
 | example_array |
 +---------------+
 | [0, 3, 6, 9]  |
 +---------------*/
```

The following returns an array using a negative value, `  -3  ` for its step size.

``` text
SELECT GENERATE_ARRAY(10, 0, -3) AS example_array;

/*---------------+
 | example_array |
 +---------------+
 | [10, 7, 4, 1] |
 +---------------*/
```

The following returns an array using the same value for the `  start_expression  ` and `  end_expression  ` .

``` text
SELECT GENERATE_ARRAY(4, 4, 10) AS example_array;

/*---------------+
 | example_array |
 +---------------+
 | [4]           |
 +---------------*/
```

The following returns an empty array, because the `  start_expression  ` is greater than the `  end_expression  ` , and the `  step_expression  ` value is positive.

``` text
SELECT GENERATE_ARRAY(10, 0, 3) AS example_array;

/*---------------+
 | example_array |
 +---------------+
 | []            |
 +---------------*/
```

The following returns a `  NULL  ` array because `  end_expression  ` is `  NULL  ` .

``` text
SELECT GENERATE_ARRAY(5, NULL, 1) AS example_array;

/*---------------+
 | example_array |
 +---------------+
 | NULL          |
 +---------------*/
```

The following returns multiple arrays.

``` text
SELECT GENERATE_ARRAY(start, 5) AS example_array
FROM UNNEST([3, 4, 5]) AS start;

/*---------------+
 | example_array |
 +---------------+
 | [3, 4, 5]     |
 | [4, 5]        |
 | [5]           |
 +---------------*/
```

## `     GENERATE_DATE_ARRAY    `

``` text
GENERATE_DATE_ARRAY(start_date, end_date[, INTERVAL INT64_expr date_part])
```

**Description**

Returns an array of dates. The `  start_date  ` and `  end_date  ` parameters determine the inclusive start and end of the array.

The `  GENERATE_DATE_ARRAY  ` function accepts the following data types as inputs:

  - `  start_date  ` must be a `  DATE  ` .
  - `  end_date  ` must be a `  DATE  ` .
  - `  INT64_expr  ` must be an `  INT64  ` .
  - `  date_part  ` must be either DAY, WEEK, MONTH, QUARTER, or YEAR.

The `  INT64_expr  ` parameter determines the increment used to generate dates. The default value for this parameter is 1 day.

This function returns an error if `  INT64_expr  ` is set to 0.

**Return Data Type**

`  ARRAY  ` containing 0 or more `  DATE  ` values.

**Examples**

The following returns an array of dates, with a default step of 1.

``` text
SELECT GENERATE_DATE_ARRAY('2016-10-05', '2016-10-08') AS example;

/*--------------------------------------------------+
 | example                                          |
 +--------------------------------------------------+
 | [2016-10-05, 2016-10-06, 2016-10-07, 2016-10-08] |
 +--------------------------------------------------*/
```

The following returns an array using a user-specified step size.

``` text
SELECT GENERATE_DATE_ARRAY(
 '2016-10-05', '2016-10-09', INTERVAL 2 DAY) AS example;

/*--------------------------------------+
 | example                              |
 +--------------------------------------+
 | [2016-10-05, 2016-10-07, 2016-10-09] |
 +--------------------------------------*/
```

The following returns an array using a negative value, `  -3  ` for its step size.

``` text
SELECT GENERATE_DATE_ARRAY('2016-10-05',
  '2016-10-01', INTERVAL -3 DAY) AS example;

/*--------------------------+
 | example                  |
 +--------------------------+
 | [2016-10-05, 2016-10-02] |
 +--------------------------*/
```

The following returns an array using the same value for the `  start_date  ` and `  end_date  ` .

``` text
SELECT GENERATE_DATE_ARRAY('2016-10-05',
  '2016-10-05', INTERVAL 8 DAY) AS example;

/*--------------+
 | example      |
 +--------------+
 | [2016-10-05] |
 +--------------*/
```

The following returns an empty array, because the `  start_date  ` is greater than the `  end_date  ` , and the `  step  ` value is positive.

``` text
SELECT GENERATE_DATE_ARRAY('2016-10-05',
  '2016-10-01', INTERVAL 1 DAY) AS example;

/*---------+
 | example |
 +---------+
 | []      |
 +---------*/
```

The following returns a `  NULL  ` array, because one of its inputs is `  NULL  ` .

``` text
SELECT GENERATE_DATE_ARRAY('2016-10-05', NULL) AS example;

/*---------+
 | example |
 +---------+
 | NULL    |
 +---------*/
```

The following returns an array of dates, using MONTH as the `  date_part  ` interval:

``` text
SELECT GENERATE_DATE_ARRAY('2016-01-01',
  '2016-12-31', INTERVAL 2 MONTH) AS example;

/*--------------------------------------------------------------------------+
 | example                                                                  |
 +--------------------------------------------------------------------------+
 | [2016-01-01, 2016-03-01, 2016-05-01, 2016-07-01, 2016-09-01, 2016-11-01] |
 +--------------------------------------------------------------------------*/
```

The following uses non-constant dates to generate an array.

``` text
SELECT GENERATE_DATE_ARRAY(date_start, date_end, INTERVAL 1 WEEK) AS date_range
FROM (
  SELECT DATE '2016-01-01' AS date_start, DATE '2016-01-31' AS date_end
  UNION ALL SELECT DATE "2016-04-01", DATE "2016-04-30"
  UNION ALL SELECT DATE "2016-07-01", DATE "2016-07-31"
  UNION ALL SELECT DATE "2016-10-01", DATE "2016-10-31"
) AS items;

/*--------------------------------------------------------------+
 | date_range                                                   |
 +--------------------------------------------------------------+
 | [2016-01-01, 2016-01-08, 2016-01-15, 2016-01-22, 2016-01-29] |
 | [2016-04-01, 2016-04-08, 2016-04-15, 2016-04-22, 2016-04-29] |
 | [2016-07-01, 2016-07-08, 2016-07-15, 2016-07-22, 2016-07-29] |
 | [2016-10-01, 2016-10-08, 2016-10-15, 2016-10-22, 2016-10-29] |
 +--------------------------------------------------------------*/
```

## Supplemental materials

### OFFSET and ORDINAL

For information about using `  OFFSET  ` and `  ORDINAL  ` with arrays, see [Array subscript operator](/spanner/docs/reference/standard-sql/operators#array_subscript_operator) and [Accessing array elements](/spanner/docs/reference/standard-sql/arrays#accessing_array_elements) .
