GoogleSQL for Spanner supports conversion. Conversion includes, but isn't limited to, casting, coercion, and supertyping.

  - Casting is explicit conversion and uses the [`  CAST()  `](/spanner/docs/reference/standard-sql/conversion_functions#cast) function.
  - Coercion is implicit conversion, which GoogleSQL performs automatically under the conditions described below.
  - A supertype is a common type to which two or more expressions can be coerced.

There are also conversions that have their own function names, such as `  PARSE_DATE()  ` . To learn more about these functions, see [Conversion functions](/spanner/docs/reference/standard-sql/conversion_functions) .

### Comparison of casting and coercion

The following table summarizes all possible cast and coercion possibilities for GoogleSQL data types. The *Coerce to* column applies to all expressions of a given data type, (for example, a column).

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>From type</th>
<th>Cast to</th>
<th>Coerce to</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       INT64      </code></td>
<td><code dir="ltr" translate="no">        BOOL       </code><br />
<code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
<code dir="ltr" translate="no">        STRING       </code><br />
<code dir="ltr" translate="no">        ENUM       </code><br />
</td>
<td><code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       NUMERIC      </code></td>
<td><code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
<code dir="ltr" translate="no">        STRING       </code><br />
</td>
<td><code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       FLOAT32      </code></td>
<td><code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
<code dir="ltr" translate="no">        STRING       </code><br />
</td>
<td><code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
<code dir="ltr" translate="no">        STRING       </code><br />
</td>
<td></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       BOOL      </code></td>
<td><code dir="ltr" translate="no">        BOOL       </code><br />
<code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        STRING       </code><br />
</td>
<td></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td><code dir="ltr" translate="no">        BOOL       </code><br />
<code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
<code dir="ltr" translate="no">        STRING       </code><br />
<code dir="ltr" translate="no">        BYTES       </code><br />
<code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
<code dir="ltr" translate="no">        ENUM       </code><br />
<code dir="ltr" translate="no">        PROTO       </code><br />
</td>
<td></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       BYTES      </code></td>
<td><code dir="ltr" translate="no">        STRING       </code><br />
<code dir="ltr" translate="no">        BYTES       </code><br />
<code dir="ltr" translate="no">        PROTO       </code><br />
</td>
<td></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       DATE      </code></td>
<td><code dir="ltr" translate="no">        STRING       </code><br />
<code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       TIMESTAMP      </code></td>
<td><code dir="ltr" translate="no">        STRING       </code><br />
<code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       ARRAY      </code></td>
<td><code dir="ltr" translate="no">        ARRAY       </code><br />
</td>
<td></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       ENUM      </code></td>
<td><code dir="ltr" translate="no">        ENUM       </code> (with the same <code dir="ltr" translate="no">        ENUM       </code> name)<br />
<code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        STRING       </code><br />
</td>
<td><code dir="ltr" translate="no">       ENUM      </code> (with the same <code dir="ltr" translate="no">       ENUM      </code> name)</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRUCT      </code></td>
<td><code dir="ltr" translate="no">        STRUCT       </code><br />
</td>
<td></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       PROTO      </code></td>
<td><code dir="ltr" translate="no">        PROTO       </code> (with the same <code dir="ltr" translate="no">        PROTO       </code> name)<br />
<code dir="ltr" translate="no">        STRING       </code><br />
<code dir="ltr" translate="no">        BYTES       </code><br />
</td>
<td><code dir="ltr" translate="no">       PROTO      </code> (with the same <code dir="ltr" translate="no">       PROTO      </code> name)</td>
</tr>
</tbody>
</table>

### Casting

Most data types can be cast from one type to another with the `  CAST  ` function. When using `  CAST  ` , a query can fail if GoogleSQL is unable to perform the cast. If you want to protect your queries from these types of errors, you can use `  SAFE_CAST  ` . To learn more about the rules for `  CAST  ` , `  SAFE_CAST  ` and other casting functions, see [Conversion functions](/spanner/docs/reference/standard-sql/conversion_functions) .

### Coercion

GoogleSQL coerces the result type of an argument expression to another type if needed to match function signatures. For example, if function `  func()  ` is defined to take a single argument of type `  FLOAT64  ` and an expression is used as an argument that has a result type of `  INT64  ` , then the result of the expression will be coerced to `  FLOAT64  ` type before `  func()  ` is computed.

### Supertypes

A supertype is a common type to which two or more expressions can be coerced. Supertypes are used with set operations such as `  UNION ALL  ` and expressions such as `  CASE  ` that expect multiple arguments with matching types. Each type has one or more supertypes, including itself, which defines its set of supertypes.

<table>
<colgroup>
<col style="width: 50%" />
<col style="width: 50%" />
</colgroup>
<thead>
<tr class="header">
<th>Input type</th>
<th>Supertypes</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       BOOL      </code></td>
<td><code dir="ltr" translate="no">        BOOL       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       INT64      </code></td>
<td><code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
<code dir="ltr" translate="no">        NUMERIC       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       FLOAT32      </code></td>
<td><code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       NUMERIC      </code></td>
<td><code dir="ltr" translate="no">        NUMERIC       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       STRING      </code></td>
<td><code dir="ltr" translate="no">        STRING       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       DATE      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       TIMESTAMP      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       ENUM      </code></td>
<td><code dir="ltr" translate="no">       ENUM      </code> with the same name. The resulting enum supertype is the one that occurred first.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       BYTES      </code></td>
<td><code dir="ltr" translate="no">        BYTES       </code><br />
</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       STRUCT      </code></td>
<td><code dir="ltr" translate="no">       STRUCT      </code> with the same field position types.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       ARRAY      </code></td>
<td><code dir="ltr" translate="no">       ARRAY      </code> with the same element types.</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       PROTO      </code></td>
<td><code dir="ltr" translate="no">       PROTO      </code> with the same name. The resulting <code dir="ltr" translate="no">       PROTO      </code> supertype is the one that occurred first. For example, the first occurrence could be in the first branch of a set operation or the first result expression in a <code dir="ltr" translate="no">       CASE      </code> statement.</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       GRAPH_ELEMENT      </code></td>
<td><code dir="ltr" translate="no">       GRAPH_ELEMENT      </code> . A graph element can be a supertype of another graph element if the following is true:
<ul>
<li>Graph element <code dir="ltr" translate="no">         a        </code> is a supertype of graph element <code dir="ltr" translate="no">         b        </code> and they're the same element kind.</li>
<li>Graph element <code dir="ltr" translate="no">         a        </code> 's property type list is a compatible superset of graph element <code dir="ltr" translate="no">         b        </code> 's property type list. This means that properties with the same name must also have the same type.</li>
</ul></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       GRAPH_PATH      </code></td>
<td><code dir="ltr" translate="no">       GRAPH_PATH      </code> . A graph path can be a supertype of another graph path if the following is true:
<ul>
<li>Graph path <code dir="ltr" translate="no">         a        </code> is a supertype of graph path <code dir="ltr" translate="no">         b        </code> if the node type for <code dir="ltr" translate="no">         a        </code> is a supertype of the node type for <code dir="ltr" translate="no">         b        </code> . In addition, the edge type for <code dir="ltr" translate="no">         a        </code> must be a supertype of the edge type for <code dir="ltr" translate="no">         b        </code> .</li>
<li>Graph path <code dir="ltr" translate="no">         a        </code> 's property type list is a compatible superset of graph path <code dir="ltr" translate="no">         b        </code> 's property type list. This means that properties with the same name must also have the same type.</li>
</ul></td>
</tr>
</tbody>
</table>

If you want to find the supertype for a set of input types, first determine the intersection of the set of supertypes for each input type. If that set is empty then the input types have no common supertype. If that set is non-empty, then the common supertype is generally the [most specific](#supertype_specificity) type in that set. Generally, the most specific type is the type with the most restrictive domain.

**Examples**

<table>
<colgroup>
<col style="width: 25%" />
<col style="width: 25%" />
<col style="width: 25%" />
<col style="width: 25%" />
</colgroup>
<thead>
<tr class="header">
<th>Input types</th>
<th>Common supertype</th>
<th>Returns</th>
<th>Notes</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        FLOAT32       </code><br />
</td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td>If you apply supertyping to <code dir="ltr" translate="no">       INT64      </code> and <code dir="ltr" translate="no">       FLOAT32      </code> , supertyping succeeds because they they share a supertype, <code dir="ltr" translate="no">       FLOAT64      </code> .</td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td>If you apply supertyping to <code dir="ltr" translate="no">       INT64      </code> and <code dir="ltr" translate="no">       FLOAT64      </code> , supertyping succeeds because they they share a supertype, <code dir="ltr" translate="no">       FLOAT64      </code> .</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        BOOL       </code><br />
</td>
<td>None</td>
<td>Error</td>
<td>If you apply supertyping to <code dir="ltr" translate="no">       INT64      </code> and <code dir="ltr" translate="no">       BOOL      </code> , supertyping fails because they don't share a common supertype.</td>
</tr>
</tbody>
</table>

#### Exact and inexact types

Numeric types can be exact or inexact. For supertyping, if all of the input types are exact types, then the resulting supertype can only be an exact type.

The following table contains a list of exact and inexact numeric data types.

<table>
<colgroup>
<col style="width: 50%" />
<col style="width: 50%" />
</colgroup>
<thead>
<tr class="header">
<th>Exact types</th>
<th>Inexact types</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        NUMERIC       </code><br />
</td>
<td><code dir="ltr" translate="no">        FLOAT32       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
</tr>
</tbody>
</table>

**Examples**

<table>
<colgroup>
<col style="width: 25%" />
<col style="width: 25%" />
<col style="width: 25%" />
<col style="width: 25%" />
</colgroup>
<thead>
<tr class="header">
<th>Input types</th>
<th>Common supertype</th>
<th>Returns</th>
<th>Notes</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">        INT64       </code><br />
<code dir="ltr" translate="no">        FLOAT64       </code><br />
</td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td>If supertyping is applied to <code dir="ltr" translate="no">       INT64      </code> and <code dir="ltr" translate="no">       FLOAT64      </code> , supertyping succeeds because there are exact and inexact numeric types being supertyped.</td>
</tr>
</tbody>
</table>

#### Types specificity

Each type has a domain of values that it supports. A type with a narrow domain is more specific than a type with a wider domain. Exact types are more specific than inexact types because inexact types have a wider range of domain values that are supported than exact types. For example, `  INT64  ` is more specific than `  FLOAT64  ` .

#### Supertypes and literals

Supertype rules for literals are more permissive than for normal expressions, and are consistent with implicit coercion rules. The following algorithm is used when the input set of types includes types related to literals:

  - If there exists non-literals in the set, find the set of common supertypes of the non-literals.
  - If there is at least one possible supertype, find the [most specific](#supertype_specificity) type to which the remaining literal types can be implicitly coerced and return that supertype. Otherwise, there is no supertype.
  - If the set only contains types related to literals, compute the supertype of the literal types.
  - If all input types are related to `  NULL  ` literals, then the resulting supertype is `  INT64  ` .
  - If no common supertype is found, an error is produced.

**Examples**

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>Input types</th>
<th>Common supertype</th>
<th>Returns</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       INT64      </code> literal<br />
<code dir="ltr" translate="no">       UINT64      </code> expression<br />
</td>
<td><code dir="ltr" translate="no">       UINT64      </code></td>
<td><code dir="ltr" translate="no">       UINT64      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       FLOAT64      </code> literal<br />
<code dir="ltr" translate="no">       FLOAT32      </code> expression<br />
</td>
<td><code dir="ltr" translate="no">       FLOAT32      </code></td>
<td><code dir="ltr" translate="no">       FLOAT32      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       INT64      </code> literal<br />
<code dir="ltr" translate="no">       FLOAT64      </code> literal<br />
</td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
<td><code dir="ltr" translate="no">       FLOAT64      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       TIMESTAMP      </code> expression<br />
<code dir="ltr" translate="no">       STRING      </code> literal<br />
</td>
<td><code dir="ltr" translate="no">       TIMESTAMP      </code></td>
<td><code dir="ltr" translate="no">       TIMESTAMP      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       NULL      </code> literal<br />
<code dir="ltr" translate="no">       NULL      </code> literal<br />
</td>
<td><code dir="ltr" translate="no">       INT64      </code></td>
<td><code dir="ltr" translate="no">       INT64      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       BOOL      </code> literal<br />
<code dir="ltr" translate="no">       TIMESTAMP      </code> literal<br />
</td>
<td>None</td>
<td>Error</td>
</tr>
</tbody>
</table>
