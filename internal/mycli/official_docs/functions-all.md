GoogleSQL is the new name for Google Standard SQL\! New name, same great SQL dialect.

This topic contains all functions supported by GoogleSQL for Spanner.

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
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#abs"><code dir="ltr" translate="no">        ABS       </code></a></td>
<td>Computes the absolute value of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#acos"><code dir="ltr" translate="no">        ACOS       </code></a></td>
<td>Computes the inverse cosine of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#acosh"><code dir="ltr" translate="no">        ACOSH       </code></a></td>
<td>Computes the inverse hyperbolic cosine of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#adddate"><code dir="ltr" translate="no">        ADDDATE       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       DATE_ADD      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#any_value"><code dir="ltr" translate="no">        ANY_VALUE       </code></a></td>
<td>Gets an expression for some row.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#approx_cosine_distance"><code dir="ltr" translate="no">        APPROX_COSINE_DISTANCE       </code></a></td>
<td>Computes the approximate cosine distance between two vectors.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#approx_dot_product"><code dir="ltr" translate="no">        APPROX_DOT_PRODUCT       </code></a></td>
<td>Computes the approximate dot product of two vectors.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#approx_euclidean_distance"><code dir="ltr" translate="no">        APPROX_EUCLIDEAN_DISTANCE       </code></a></td>
<td>Computes the approximate Euclidean distance between two vectors.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array"><code dir="ltr" translate="no">        ARRAY       </code></a></td>
<td>Produces an array with one element for each row in a subquery.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#array_agg"><code dir="ltr" translate="no">        ARRAY_AGG       </code></a></td>
<td>Gets an array of values.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#array_concat"><code dir="ltr" translate="no">        ARRAY_CONCAT       </code></a></td>
<td>Concatenates one or more arrays with the same element type into a single array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#array_concat_agg"><code dir="ltr" translate="no">        ARRAY_CONCAT_AGG       </code></a></td>
<td>Concatenates arrays and returns a single array as a result.</td>
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
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#asin"><code dir="ltr" translate="no">        ASIN       </code></a></td>
<td>Computes the inverse sine of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#asinh"><code dir="ltr" translate="no">        ASINH       </code></a></td>
<td>Computes the inverse hyperbolic sine of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#atan"><code dir="ltr" translate="no">        ATAN       </code></a></td>
<td>Computes the inverse tangent of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#atan2"><code dir="ltr" translate="no">        ATAN2       </code></a></td>
<td>Computes the inverse tangent of <code dir="ltr" translate="no">       X/Y      </code> , using the signs of <code dir="ltr" translate="no">       X      </code> and <code dir="ltr" translate="no">       Y      </code> to determine the quadrant.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#atanh"><code dir="ltr" translate="no">        ATANH       </code></a></td>
<td>Computes the inverse hyperbolic tangent of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#avg"><code dir="ltr" translate="no">        AVG       </code></a></td>
<td>Gets the average of non- <code dir="ltr" translate="no">       NULL      </code> values.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#bit_and"><code dir="ltr" translate="no">        BIT_AND       </code></a></td>
<td>Performs a bitwise AND operation on an expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/bit_functions#bit_count"><code dir="ltr" translate="no">        BIT_COUNT       </code></a></td>
<td>Gets the number of bits that are set in an input expression.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#bit_or"><code dir="ltr" translate="no">        BIT_OR       </code></a></td>
<td>Performs a bitwise OR operation on an expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/bit_functions#bit_reverse"><code dir="ltr" translate="no">        BIT_REVERSE       </code></a></td>
<td>Reverses the bits in an integer.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#bit_xor"><code dir="ltr" translate="no">        BIT_XOR       </code></a></td>
<td>Performs a bitwise XOR operation on an expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#bool_for_json"><code dir="ltr" translate="no">        BOOL       </code></a></td>
<td>Converts a JSON boolean to a SQL <code dir="ltr" translate="no">       BOOL      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#bool_array_for_json"><code dir="ltr" translate="no">        BOOL_ARRAY       </code></a></td>
<td>Converts a JSON array of booleans to a SQL <code dir="ltr" translate="no">       ARRAY&lt;BOOL&gt;      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#byte_length"><code dir="ltr" translate="no">        BYTE_LENGTH       </code></a></td>
<td>Gets the number of <code dir="ltr" translate="no">       BYTES      </code> in a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/conversion_functions#cast"><code dir="ltr" translate="no">        CAST       </code></a></td>
<td>Convert the results of an expression to the given type.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#ceil"><code dir="ltr" translate="no">        CEIL       </code></a></td>
<td>Gets the smallest integral value that isn't less than <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#ceiling"><code dir="ltr" translate="no">        CEILING       </code></a></td>
<td>Synonym of <code dir="ltr" translate="no">       CEIL      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#char_length"><code dir="ltr" translate="no">        CHAR_LENGTH       </code></a></td>
<td>Gets the number of characters in a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#character_length"><code dir="ltr" translate="no">        CHARACTER_LENGTH       </code></a></td>
<td>Synonym for <code dir="ltr" translate="no">       CHAR_LENGTH      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#code_points_to_bytes"><code dir="ltr" translate="no">        CODE_POINTS_TO_BYTES       </code></a></td>
<td>Converts an array of extended ASCII code points to a <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#code_points_to_string"><code dir="ltr" translate="no">        CODE_POINTS_TO_STRING       </code></a></td>
<td>Converts an array of extended ASCII code points to a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#concat"><code dir="ltr" translate="no">        CONCAT       </code></a></td>
<td>Concatenates one or more <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> values into a single result.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#cos"><code dir="ltr" translate="no">        COS       </code></a></td>
<td>Computes the cosine of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#cosh"><code dir="ltr" translate="no">        COSH       </code></a></td>
<td>Computes the hyperbolic cosine of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#cosine_distance"><code dir="ltr" translate="no">        COSINE_DISTANCE       </code></a></td>
<td>Computes the cosine distance between two vectors.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#count"><code dir="ltr" translate="no">        COUNT       </code></a></td>
<td>Gets the number of rows in the input, or the number of rows with an expression evaluated to any value other than <code dir="ltr" translate="no">       NULL      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#countif"><code dir="ltr" translate="no">        COUNTIF       </code></a></td>
<td>Gets the number of <code dir="ltr" translate="no">       TRUE      </code> values for an expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#current_date"><code dir="ltr" translate="no">        CURRENT_DATE       </code></a></td>
<td>Returns the current date as a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#current_timestamp"><code dir="ltr" translate="no">        CURRENT_TIMESTAMP       </code></a></td>
<td>Returns the current date and time as a <code dir="ltr" translate="no">       TIMESTAMP      </code> object.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date"><code dir="ltr" translate="no">        DATE       </code></a></td>
<td>Constructs a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_add"><code dir="ltr" translate="no">        DATE_ADD       </code></a></td>
<td>Adds a specified time interval to a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_diff"><code dir="ltr" translate="no">        DATE_DIFF       </code></a></td>
<td>Gets the number of unit boundaries between two <code dir="ltr" translate="no">       DATE      </code> values at a particular time granularity.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_from_unix_date"><code dir="ltr" translate="no">        DATE_FROM_UNIX_DATE       </code></a></td>
<td>Interprets an <code dir="ltr" translate="no">       INT64      </code> expression as the number of days since 1970-01-01.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_sub"><code dir="ltr" translate="no">        DATE_SUB       </code></a></td>
<td>Subtracts a specified time interval from a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_trunc"><code dir="ltr" translate="no">        DATE_TRUNC       </code></a></td>
<td>Truncates a <code dir="ltr" translate="no">       DATE      </code> value at a particular granularity.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#debug_tokenlist"><code dir="ltr" translate="no">        DEBUG_TOKENLIST       </code></a></td>
<td>Displays a human-readable representation of tokens present in the <code dir="ltr" translate="no">       TOKENLIST      </code> value for debugging purposes.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#destination_node_id"><code dir="ltr" translate="no">        DESTINATION_NODE_ID       </code></a></td>
<td>Gets a unique identifier of a graph edge's destination node.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#div"><code dir="ltr" translate="no">        DIV       </code></a></td>
<td>Divides integer <code dir="ltr" translate="no">       X      </code> by integer <code dir="ltr" translate="no">       Y      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#dot_product"><code dir="ltr" translate="no">        DOT_PRODUCT       </code></a></td>
<td>Computes the dot product of two vectors.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#double_for_json"><code dir="ltr" translate="no">        FLOAT64       </code></a></td>
<td>Converts a JSON number to a SQL <code dir="ltr" translate="no">       FLOAT64      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#double_array_for_json"><code dir="ltr" translate="no">        FLOAT64_ARRAY       </code></a></td>
<td>Converts a JSON array of numbers to a SQL <code dir="ltr" translate="no">       ARRAY&lt;FLOAT64&gt;      </code> value.</td>
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
<td><a href="/spanner/docs/reference/standard-sql/string_functions#ends_with"><code dir="ltr" translate="no">        ENDS_WITH       </code></a></td>
<td>Checks if a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value is the suffix of another value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/debugging_functions#error"><code dir="ltr" translate="no">        ERROR       </code></a></td>
<td>Produces an error with a custom error message.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#exp"><code dir="ltr" translate="no">        EXP       </code></a></td>
<td>Computes <code dir="ltr" translate="no">       e      </code> to the power of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#extract"><code dir="ltr" translate="no">        EXTRACT       </code></a></td>
<td>Extracts part of a date from a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/interval_functions#extract"><code dir="ltr" translate="no">        EXTRACT       </code></a></td>
<td>Extracts part of an <code dir="ltr" translate="no">       INTERVAL      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#extract"><code dir="ltr" translate="no">        EXTRACT       </code></a></td>
<td>Extracts part of a <code dir="ltr" translate="no">       TIMESTAMP      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#euclidean_distance"><code dir="ltr" translate="no">        EUCLIDEAN_DISTANCE       </code></a></td>
<td>Computes the Euclidean distance between two vectors.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/hash_functions#farm_fingerprint"><code dir="ltr" translate="no">        FARM_FINGERPRINT       </code></a></td>
<td>Computes the fingerprint of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value, using the FarmHash Fingerprint64 algorithm.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#float_for_json"><code dir="ltr" translate="no">        FLOAT32       </code></a></td>
<td>Converts a JSON number to a SQL <code dir="ltr" translate="no">       FLOAT32      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#float_array_for_json"><code dir="ltr" translate="no">        FLOAT32_ARRAY       </code></a></td>
<td>Converts a JSON array of numbers to a SQL <code dir="ltr" translate="no">       ARRAY&lt;FLOAT32&gt;      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#floor"><code dir="ltr" translate="no">        FLOOR       </code></a></td>
<td>Gets the largest integral value that isn't greater than <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#format_date"><code dir="ltr" translate="no">        FORMAT_DATE       </code></a></td>
<td>Formats a <code dir="ltr" translate="no">       DATE      </code> value according to a specified format string.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#format_timestamp"><code dir="ltr" translate="no">        FORMAT_TIMESTAMP       </code></a></td>
<td>Formats a <code dir="ltr" translate="no">       TIMESTAMP      </code> value according to the specified format string.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#format_string"><code dir="ltr" translate="no">        FORMAT       </code></a></td>
<td>Formats data and produces the results as a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#from_base32"><code dir="ltr" translate="no">        FROM_BASE32       </code></a></td>
<td>Converts a base32-encoded <code dir="ltr" translate="no">       STRING      </code> value into a <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#from_base64"><code dir="ltr" translate="no">        FROM_BASE64       </code></a></td>
<td>Converts a base64-encoded <code dir="ltr" translate="no">       STRING      </code> value into a <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#from_hex"><code dir="ltr" translate="no">        FROM_HEX       </code></a></td>
<td>Converts a hexadecimal-encoded <code dir="ltr" translate="no">       STRING      </code> value into a <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#generate_array"><code dir="ltr" translate="no">        GENERATE_ARRAY       </code></a></td>
<td>Generates an array of values in a range.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#generate_date_array"><code dir="ltr" translate="no">        GENERATE_DATE_ARRAY       </code></a></td>
<td>Generates an array of dates in a range.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/utility-functions#generate_uuid"><code dir="ltr" translate="no">        GENERATE_UUID       </code></a></td>
<td>Produces a random universally unique identifier (UUID) as a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/sequence_functions#get_internal_sequence_state"><code dir="ltr" translate="no">        GET_INTERNAL_SEQUENCE_STATE       </code></a></td>
<td>Gets the current sequence internal counter before bit reversal.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/sequence_functions#get_next_sequence_value"><code dir="ltr" translate="no">        GET_NEXT_SEQUENCE_VALUE       </code></a></td>
<td>Takes in a sequence identifier and returns the next value. This function is only allowed in read-write transactions.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#greatest"><code dir="ltr" translate="no">        GREATEST       </code></a></td>
<td>Gets the greatest value among <code dir="ltr" translate="no">       X1,...,XN      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#ieee_divide"><code dir="ltr" translate="no">        IEEE_DIVIDE       </code></a></td>
<td>Divides <code dir="ltr" translate="no">       X      </code> by <code dir="ltr" translate="no">       Y      </code> , but doesn't generate errors for division by zero or overflow.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#int64_for_json"><code dir="ltr" translate="no">        INT64       </code></a></td>
<td>Converts a JSON number to a SQL <code dir="ltr" translate="no">       INT64      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#int64_array_for_json"><code dir="ltr" translate="no">        INT64_ARRAY       </code></a></td>
<td>Converts a JSON array of numbers to a SQL <code dir="ltr" translate="no">       ARRAY&lt;INT64&gt;      </code> value.</td>
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
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#is_inf"><code dir="ltr" translate="no">        IS_INF       </code></a></td>
<td>Checks if <code dir="ltr" translate="no">       X      </code> is positive or negative infinity.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#is_nan"><code dir="ltr" translate="no">        IS_NAN       </code></a></td>
<td>Checks if <code dir="ltr" translate="no">       X      </code> is a <code dir="ltr" translate="no">       NaN      </code> value.</td>
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
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_array"><code dir="ltr" translate="no">        JSON_ARRAY       </code></a></td>
<td>Creates a JSON array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_array_append"><code dir="ltr" translate="no">        JSON_ARRAY_APPEND       </code></a></td>
<td>Appends JSON data to the end of a JSON array.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_array_insert"><code dir="ltr" translate="no">        JSON_ARRAY_INSERT       </code></a></td>
<td>Inserts JSON data into a JSON array.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_contains"><code dir="ltr" translate="no">        JSON_CONTAINS       </code></a></td>
<td>Checks if a JSON document contains another JSON document.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_keys"><code dir="ltr" translate="no">        JSON_KEYS       </code></a></td>
<td>Extracts unique JSON keys from a JSON expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_object"><code dir="ltr" translate="no">        JSON_OBJECT       </code></a></td>
<td>Creates a JSON object.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_query"><code dir="ltr" translate="no">        JSON_QUERY       </code></a></td>
<td>Extracts a JSON value and converts it to a SQL JSON-formatted <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       JSON      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_query_array"><code dir="ltr" translate="no">        JSON_QUERY_ARRAY       </code></a></td>
<td>Extracts a JSON array and converts it to a SQL <code dir="ltr" translate="no">       ARRAY&lt;JSON-formatted STRING&gt;      </code> or <code dir="ltr" translate="no">       ARRAY&lt;JSON&gt;      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_remove"><code dir="ltr" translate="no">        JSON_REMOVE       </code></a></td>
<td>Produces JSON with the specified JSON data removed.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_set"><code dir="ltr" translate="no">        JSON_SET       </code></a></td>
<td>Inserts or replaces JSON data.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_strip_nulls"><code dir="ltr" translate="no">        JSON_STRIP_NULLS       </code></a></td>
<td>Removes JSON nulls from JSON objects and JSON arrays.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_type"><code dir="ltr" translate="no">        JSON_TYPE       </code></a></td>
<td>Gets the JSON type of the outermost JSON value and converts the name of this type to a SQL <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_value"><code dir="ltr" translate="no">        JSON_VALUE       </code></a></td>
<td>Extracts a JSON scalar value and converts it to a SQL <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#json_value_array"><code dir="ltr" translate="no">        JSON_VALUE_ARRAY       </code></a></td>
<td>Extracts a JSON array of scalar values and converts it to a SQL <code dir="ltr" translate="no">       ARRAY&lt;STRING&gt;      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/interval_functions#justify_days"><code dir="ltr" translate="no">        JUSTIFY_DAYS       </code></a></td>
<td>Normalizes the day part of an <code dir="ltr" translate="no">       INTERVAL      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/interval_functions#justify_hours"><code dir="ltr" translate="no">        JUSTIFY_HOURS       </code></a></td>
<td>Normalizes the time part of an <code dir="ltr" translate="no">       INTERVAL      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/interval_functions#justify_interval"><code dir="ltr" translate="no">        JUSTIFY_INTERVAL       </code></a></td>
<td>Normalizes the day and time parts of an <code dir="ltr" translate="no">       INTERVAL      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#labels"><code dir="ltr" translate="no">        LABELS       </code></a></td>
<td>Gets the labels associated with a graph element.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_bool"><code dir="ltr" translate="no">        LAX_BOOL       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       BOOL      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_double"><code dir="ltr" translate="no">        LAX_FLOAT64       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       FLOAT64      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_int64"><code dir="ltr" translate="no">        LAX_INT64       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       INT64      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#lax_string"><code dir="ltr" translate="no">        LAX_STRING       </code></a></td>
<td>Attempts to convert a JSON value to a SQL <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#lcase"><code dir="ltr" translate="no">        LCASE       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       LOWER      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#least"><code dir="ltr" translate="no">        LEAST       </code></a></td>
<td>Gets the least value among <code dir="ltr" translate="no">       X1,...,XN      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#length"><code dir="ltr" translate="no">        LENGTH       </code></a></td>
<td>Gets the length of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#ln"><code dir="ltr" translate="no">        LN       </code></a></td>
<td>Computes the natural logarithm of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#log"><code dir="ltr" translate="no">        LOG       </code></a></td>
<td>Computes the natural logarithm of <code dir="ltr" translate="no">       X      </code> or the logarithm of <code dir="ltr" translate="no">       X      </code> to base <code dir="ltr" translate="no">       Y      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#log10"><code dir="ltr" translate="no">        LOG10       </code></a></td>
<td>Computes the natural logarithm of <code dir="ltr" translate="no">       X      </code> to base 10.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#logical_and"><code dir="ltr" translate="no">        LOGICAL_AND       </code></a></td>
<td>Gets the logical AND of all non- <code dir="ltr" translate="no">       NULL      </code> expressions.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#logical_or"><code dir="ltr" translate="no">        LOGICAL_OR       </code></a></td>
<td>Gets the logical OR of all non- <code dir="ltr" translate="no">       NULL      </code> expressions.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#lower"><code dir="ltr" translate="no">        LOWER       </code></a></td>
<td>Formats alphabetic characters in a <code dir="ltr" translate="no">       STRING      </code> value as lowercase.<br />
<br />
Formats ASCII characters in a <code dir="ltr" translate="no">       BYTES      </code> value as lowercase.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#lpad"><code dir="ltr" translate="no">        LPAD       </code></a></td>
<td>Prepends a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value with a pattern.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#ltrim"><code dir="ltr" translate="no">        LTRIM       </code></a></td>
<td>Identical to the <code dir="ltr" translate="no">       TRIM      </code> function, but only removes leading characters.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/interval_functions#make_interval"><code dir="ltr" translate="no">        MAKE_INTERVAL       </code></a></td>
<td>Constructs an <code dir="ltr" translate="no">       INTERVAL      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#max"><code dir="ltr" translate="no">        MAX       </code></a></td>
<td>Gets the maximum non- <code dir="ltr" translate="no">       NULL      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#min"><code dir="ltr" translate="no">        MIN       </code></a></td>
<td>Gets the minimum non- <code dir="ltr" translate="no">       NULL      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/ml-functions#mlpredict"><code dir="ltr" translate="no">        ML.PREDICT       </code></a></td>
<td>Apply ML computations defined by a model to each row of an input relation.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#mod"><code dir="ltr" translate="no">        MOD       </code></a></td>
<td>Gets the remainder of the division of <code dir="ltr" translate="no">       X      </code> by <code dir="ltr" translate="no">       Y      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#nethost"><code dir="ltr" translate="no">        NET.HOST       </code></a></td>
<td>Gets the hostname from a URL.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netip_from_string"><code dir="ltr" translate="no">        NET.IP_FROM_STRING       </code></a></td>
<td>Converts an IPv4 or IPv6 address from a <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       BYTES      </code> value in network byte order.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netip_net_mask"><code dir="ltr" translate="no">        NET.IP_NET_MASK       </code></a></td>
<td>Gets a network mask.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netip_to_string"><code dir="ltr" translate="no">        NET.IP_TO_STRING       </code></a></td>
<td>Converts an IPv4 or IPv6 address from a <code dir="ltr" translate="no">       BYTES      </code> value in network byte order to a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netip_trunc"><code dir="ltr" translate="no">        NET.IP_TRUNC       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> IPv4 or IPv6 address in network byte order to a <code dir="ltr" translate="no">       BYTES      </code> subnet address.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netipv4_from_int64"><code dir="ltr" translate="no">        NET.IPV4_FROM_INT64       </code></a></td>
<td>Converts an IPv4 address from an <code dir="ltr" translate="no">       INT64      </code> value to a <code dir="ltr" translate="no">       BYTES      </code> value in network byte order.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netipv4_to_int64"><code dir="ltr" translate="no">        NET.IPV4_TO_INT64       </code></a></td>
<td>Converts an IPv4 address from a <code dir="ltr" translate="no">       BYTES      </code> value in network byte order to an <code dir="ltr" translate="no">       INT64      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netpublic_suffix"><code dir="ltr" translate="no">        NET.PUBLIC_SUFFIX       </code></a></td>
<td>Gets the public suffix from a URL.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netreg_domain"><code dir="ltr" translate="no">        NET.REG_DOMAIN       </code></a></td>
<td>Gets the registered or registrable domain from a URL.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/net_functions#netsafe_ip_from_string"><code dir="ltr" translate="no">        NET.SAFE_IP_FROM_STRING       </code></a></td>
<td>Similar to the <code dir="ltr" translate="no">       NET.IP_FROM_STRING      </code> , but returns <code dir="ltr" translate="no">       NULL      </code> instead of producing an error if the input is invalid.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/utility-functions#new_uuid"><code dir="ltr" translate="no">        NEW_UUID       </code></a></td>
<td>Produces a random universally unique identifier (UUID) as a <code dir="ltr" translate="no">       UUID      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#nodes"><code dir="ltr" translate="no">        NODES       </code></a></td>
<td>Gets the nodes in a graph path. The resulting array retains the original order in the graph path.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#normalize"><code dir="ltr" translate="no">        NORMALIZE       </code></a></td>
<td>Case-sensitively normalizes the characters in a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#normalize_and_casefold"><code dir="ltr" translate="no">        NORMALIZE_AND_CASEFOLD       </code></a></td>
<td>Case-insensitively normalizes the characters in a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#octet_length"><code dir="ltr" translate="no">        OCTET_LENGTH       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       BYTE_LENGTH      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#parse_date"><code dir="ltr" translate="no">        PARSE_DATE       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#parse_json"><code dir="ltr" translate="no">        PARSE_JSON       </code></a></td>
<td>Converts a JSON-formatted <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       JSON      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#parse_timestamp"><code dir="ltr" translate="no">        PARSE_TIMESTAMP       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       TIMESTAMP      </code> value.</td>
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
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#pending_commit_timestamp"><code dir="ltr" translate="no">        PENDING_COMMIT_TIMESTAMP       </code></a></td>
<td>Write a pending commit timestamp.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#pow"><code dir="ltr" translate="no">        POW       </code></a></td>
<td>Produces the value of <code dir="ltr" translate="no">       X      </code> raised to the power of <code dir="ltr" translate="no">       Y      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#power"><code dir="ltr" translate="no">        POWER       </code></a></td>
<td>Synonym of <code dir="ltr" translate="no">       POW      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#property_names"><code dir="ltr" translate="no">        PROPERTY_NAMES       </code></a></td>
<td>Gets the property names associated with a graph element.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#regexp_contains"><code dir="ltr" translate="no">        REGEXP_CONTAINS       </code></a></td>
<td>Checks if a value is a partial match for a regular expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#regexp_extract"><code dir="ltr" translate="no">        REGEXP_EXTRACT       </code></a></td>
<td>Produces a substring that matches a regular expression.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#regexp_extract_all"><code dir="ltr" translate="no">        REGEXP_EXTRACT_ALL       </code></a></td>
<td>Produces an array of all substrings that match a regular expression.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#regexp_replace"><code dir="ltr" translate="no">        REGEXP_REPLACE       </code></a></td>
<td>Produces a <code dir="ltr" translate="no">       STRING      </code> value where all substrings that match a regular expression are replaced with a specified value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#repeat"><code dir="ltr" translate="no">        REPEAT       </code></a></td>
<td>Produces a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value that consists of an original value, repeated.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#replace"><code dir="ltr" translate="no">        REPLACE       </code></a></td>
<td>Replaces all occurrences of a pattern with another pattern in a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/protocol_buffer_functions#replace_fields"><code dir="ltr" translate="no">        REPLACE_FIELDS       </code></a></td>
<td>Replaces the values in one or more protocol buffer fields.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#reverse"><code dir="ltr" translate="no">        REVERSE       </code></a></td>
<td>Reverses a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#round"><code dir="ltr" translate="no">        ROUND       </code></a></td>
<td>Rounds <code dir="ltr" translate="no">       X      </code> to the nearest integer or rounds <code dir="ltr" translate="no">       X      </code> to <code dir="ltr" translate="no">       N      </code> decimal places after the decimal point.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#rpad"><code dir="ltr" translate="no">        RPAD       </code></a></td>
<td>Appends a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value with a pattern.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#rtrim"><code dir="ltr" translate="no">        RTRIM       </code></a></td>
<td>Identical to the <code dir="ltr" translate="no">       TRIM      </code> function, but only removes trailing characters.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#safe_add"><code dir="ltr" translate="no">        SAFE_ADD       </code></a></td>
<td>Equivalent to the addition operator ( <code dir="ltr" translate="no">       X + Y      </code> ), but returns <code dir="ltr" translate="no">       NULL      </code> if overflow occurs.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/conversion_functions#safe_casting"><code dir="ltr" translate="no">        SAFE_CAST       </code></a></td>
<td>Similar to the <code dir="ltr" translate="no">       CAST      </code> function, but returns <code dir="ltr" translate="no">       NULL      </code> when a runtime error is produced.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#safe_convert_bytes_to_string"><code dir="ltr" translate="no">        SAFE_CONVERT_BYTES_TO_STRING       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a <code dir="ltr" translate="no">       STRING      </code> value and replace any invalid UTF-8 characters with the Unicode replacement character, <code dir="ltr" translate="no">       U+FFFD      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#safe_divide"><code dir="ltr" translate="no">        SAFE_DIVIDE       </code></a></td>
<td>Equivalent to the division operator ( <code dir="ltr" translate="no">       X / Y      </code> ), but returns <code dir="ltr" translate="no">       NULL      </code> if an error occurs.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#safe_multiply"><code dir="ltr" translate="no">        SAFE_MULTIPLY       </code></a></td>
<td>Equivalent to the multiplication operator ( <code dir="ltr" translate="no">       X * Y      </code> ), but returns <code dir="ltr" translate="no">       NULL      </code> if overflow occurs.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#safe_negate"><code dir="ltr" translate="no">        SAFE_NEGATE       </code></a></td>
<td>Equivalent to the unary minus operator ( <code dir="ltr" translate="no">       -X      </code> ), but returns <code dir="ltr" translate="no">       NULL      </code> if overflow occurs.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#safe_subtract"><code dir="ltr" translate="no">        SAFE_SUBTRACT       </code></a></td>
<td>Equivalent to the subtraction operator ( <code dir="ltr" translate="no">       X - Y      </code> ), but returns <code dir="ltr" translate="no">       NULL      </code> if overflow occurs.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#safe_to_json"><code dir="ltr" translate="no">        SAFE_TO_JSON       </code></a></td>
<td>Similar to the `TO_JSON` function, but for each unsupported field in the input argument, produces a JSON null instead of an error.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#score"><code dir="ltr" translate="no">        SCORE       </code></a></td>
<td>Calculates a relevance score of a <code dir="ltr" translate="no">       TOKENLIST      </code> for a full-text search query. The higher the score, the stronger the match.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#score_ngrams"><code dir="ltr" translate="no">        SCORE_NGRAMS       </code></a></td>
<td>Calculates a relevance score of a <code dir="ltr" translate="no">       TOKENLIST      </code> for a fuzzy search. The higher the score, the stronger the match.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#search_fulltext"><code dir="ltr" translate="no">        SEARCH       </code></a></td>
<td>Returns <code dir="ltr" translate="no">       TRUE      </code> if a full-text search query matches tokens.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#search_ngrams"><code dir="ltr" translate="no">        SEARCH_NGRAMS       </code></a></td>
<td>Checks whether enough n-grams match the tokens in a fuzzy search.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#search_substring"><code dir="ltr" translate="no">        SEARCH_SUBSTRING       </code></a></td>
<td>Returns <code dir="ltr" translate="no">       TRUE      </code> if a substring query matches tokens.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/hash_functions#sha1"><code dir="ltr" translate="no">        SHA1       </code></a></td>
<td>Computes the hash of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value, using the SHA-1 algorithm.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/hash_functions#sha256"><code dir="ltr" translate="no">        SHA256       </code></a></td>
<td>Computes the hash of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value, using the SHA-256 algorithm.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/hash_functions#sha512"><code dir="ltr" translate="no">        SHA512       </code></a></td>
<td>Computes the hash of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value, using the SHA-512 algorithm.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#sign"><code dir="ltr" translate="no">        SIGN       </code></a></td>
<td>Produces -1 , 0, or +1 for negative, zero, and positive arguments respectively.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#sin"><code dir="ltr" translate="no">        SIN       </code></a></td>
<td>Computes the sine of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#sinh"><code dir="ltr" translate="no">        SINH       </code></a></td>
<td>Computes the hyperbolic sine of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#snippet"><code dir="ltr" translate="no">        SNIPPET       </code></a></td>
<td>Gets a list of snippets that match a full-text search query.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/graph-sql-functions#source_node_id"><code dir="ltr" translate="no">        SOURCE_NODE_ID       </code></a></td>
<td>Gets a unique identifier of a graph edge's source node.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#soundex"><code dir="ltr" translate="no">        SOUNDEX       </code></a></td>
<td>Gets the Soundex codes for words in a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#split"><code dir="ltr" translate="no">        SPLIT       </code></a></td>
<td>Splits a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value, using a delimiter.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#split_substr"><code dir="ltr" translate="no">        SPLIT_SUBSTR       </code></a></td>
<td>Returns the substring from an input string that's determined by a delimiter, a location that indicates the first split of the substring to return, and the number of splits to include.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#sqrt"><code dir="ltr" translate="no">        SQRT       </code></a></td>
<td>Computes the square root of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#starts_with"><code dir="ltr" translate="no">        STARTS_WITH       </code></a></td>
<td>Checks if a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value is a prefix of another value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#stddev"><code dir="ltr" translate="no">        STDDEV       </code></a></td>
<td>An alias of the <code dir="ltr" translate="no">       STDDEV_SAMP      </code> function.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#stddev_samp"><code dir="ltr" translate="no">        STDDEV_SAMP       </code></a></td>
<td>Computes the sample (unbiased) standard deviation of the values.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#string_for_json"><code dir="ltr" translate="no">        STRING       </code> (JSON)</a></td>
<td>Converts a JSON string to a SQL <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#string_array_for_json"><code dir="ltr" translate="no">        STRING_ARRAY       </code></a></td>
<td>Converts a JSON array of strings to a SQL <code dir="ltr" translate="no">       ARRAY&lt;STRING&gt;      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#string"><code dir="ltr" translate="no">        STRING       </code> (Timestamp)</a></td>
<td>Converts a <code dir="ltr" translate="no">       TIMESTAMP      </code> value to a <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#string_agg"><code dir="ltr" translate="no">        STRING_AGG       </code></a></td>
<td>Concatenates non- <code dir="ltr" translate="no">       NULL      </code> <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> values.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#strpos"><code dir="ltr" translate="no">        STRPOS       </code></a></td>
<td>Finds the position of the first occurrence of a subvalue inside another value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#subdate"><code dir="ltr" translate="no">        SUBDATE       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       DATE_SUB      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#substr"><code dir="ltr" translate="no">        SUBSTR       </code></a></td>
<td>Gets a portion of a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#substring"><code dir="ltr" translate="no">        SUBSTRING       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       SUBSTR      </code></td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/aggregate_functions#sum"><code dir="ltr" translate="no">        SUM       </code></a></td>
<td>Gets the sum of non- <code dir="ltr" translate="no">       NULL      </code> values.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#tan"><code dir="ltr" translate="no">        TAN       </code></a></td>
<td>Computes the tangent of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#tanh"><code dir="ltr" translate="no">        TANH       </code></a></td>
<td>Computes the hyperbolic tangent of <code dir="ltr" translate="no">       X      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp"><code dir="ltr" translate="no">        TIMESTAMP       </code></a></td>
<td>Constructs a <code dir="ltr" translate="no">       TIMESTAMP      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp_add"><code dir="ltr" translate="no">        TIMESTAMP_ADD       </code></a></td>
<td>Adds a specified time interval to a <code dir="ltr" translate="no">       TIMESTAMP      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp_diff"><code dir="ltr" translate="no">        TIMESTAMP_DIFF       </code></a></td>
<td>Gets the number of unit boundaries between two <code dir="ltr" translate="no">       TIMESTAMP      </code> values at a particular time granularity.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp_micros"><code dir="ltr" translate="no">        TIMESTAMP_MICROS       </code></a></td>
<td>Converts the number of microseconds since 1970-01-01 00:00:00 UTC to a <code dir="ltr" translate="no">       TIMESTAMP      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp_millis"><code dir="ltr" translate="no">        TIMESTAMP_MILLIS       </code></a></td>
<td>Converts the number of milliseconds since 1970-01-01 00:00:00 UTC to a <code dir="ltr" translate="no">       TIMESTAMP      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp_seconds"><code dir="ltr" translate="no">        TIMESTAMP_SECONDS       </code></a></td>
<td>Converts the number of seconds since 1970-01-01 00:00:00 UTC to a <code dir="ltr" translate="no">       TIMESTAMP      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp_sub"><code dir="ltr" translate="no">        TIMESTAMP_SUB       </code></a></td>
<td>Subtracts a specified time interval from a <code dir="ltr" translate="no">       TIMESTAMP      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#timestamp_trunc"><code dir="ltr" translate="no">        TIMESTAMP_TRUNC       </code></a></td>
<td>Truncates a <code dir="ltr" translate="no">       TIMESTAMP      </code> value at a particular granularity.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_base32"><code dir="ltr" translate="no">        TO_BASE32       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a base32-encoded <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_base64"><code dir="ltr" translate="no">        TO_BASE64       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a base64-encoded <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_code_points"><code dir="ltr" translate="no">        TO_CODE_POINTS       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value into an array of extended ASCII code points.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#to_hex"><code dir="ltr" translate="no">        TO_HEX       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       BYTES      </code> value to a hexadecimal <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#to_json"><code dir="ltr" translate="no">        TO_JSON       </code></a></td>
<td>Converts a SQL value to a JSON value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/json_functions#to_json_string"><code dir="ltr" translate="no">        TO_JSON_STRING       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       JSON      </code> value to a SQL JSON-formatted <code dir="ltr" translate="no">       STRING      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#token"><code dir="ltr" translate="no">        TOKEN       </code></a></td>
<td>Constructs an exact match <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing a <code dir="ltr" translate="no">       BYTE      </code> or <code dir="ltr" translate="no">       STRING      </code> value verbatim to accelerate exact match expressions in SQL.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_bool"><code dir="ltr" translate="no">        TOKENIZE_BOOL       </code></a></td>
<td>Constructs a boolean <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing a <code dir="ltr" translate="no">       BOOL      </code> value to accelerate boolean match expressions in SQL.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_fulltext"><code dir="ltr" translate="no">        TOKENIZE_FULLTEXT       </code></a></td>
<td>Constructs a full-text <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing text for full-text matching.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_json"><code dir="ltr" translate="no">        TOKENIZE_JSON       </code></a></td>
<td>Constructs a JSON <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing a <code dir="ltr" translate="no">       JSON      </code> value to accelerate JSON predicate expressions in SQL.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_ngrams"><code dir="ltr" translate="no">        TOKENIZE_NGRAMS       </code></a></td>
<td>Constructs an n-gram <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing a <code dir="ltr" translate="no">       STRING      </code> value for matching n-grams.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_number"><code dir="ltr" translate="no">        TOKENIZE_NUMBER       </code></a></td>
<td>Constructs a numeric <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing numeric values to accelerate numeric comparison expressions in SQL.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_substring"><code dir="ltr" translate="no">        TOKENIZE_SUBSTRING       </code></a></td>
<td>Constructs a substring <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing text for substring matching.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenlist_concat"><code dir="ltr" translate="no">        TOKENLIST_CONCAT       </code></a></td>
<td>Constructs a <code dir="ltr" translate="no">       TOKENLIST      </code> value by concatenating one or more <code dir="ltr" translate="no">       TOKENLIST      </code> values.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#trim"><code dir="ltr" translate="no">        TRIM       </code></a></td>
<td>Removes the specified leading and trailing Unicode code points or bytes from a <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/mathematical_functions#trunc"><code dir="ltr" translate="no">        TRUNC       </code></a></td>
<td>Rounds a number like <code dir="ltr" translate="no">       ROUND(X)      </code> or <code dir="ltr" translate="no">       ROUND(X, N)      </code> , but always rounds towards zero and never overflows.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#ucase"><code dir="ltr" translate="no">        UCASE       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       UPPER      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#unix_date"><code dir="ltr" translate="no">        UNIX_DATE       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       DATE      </code> value to the number of days since 1970-01-01.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#unix_micros"><code dir="ltr" translate="no">        UNIX_MICROS       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       TIMESTAMP      </code> value to the number of microseconds since 1970-01-01 00:00:00 UTC.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#unix_millis"><code dir="ltr" translate="no">        UNIX_MILLIS       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       TIMESTAMP      </code> value to the number of milliseconds since 1970-01-01 00:00:00 UTC.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#unix_seconds"><code dir="ltr" translate="no">        UNIX_SECONDS       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       TIMESTAMP      </code> value to the number of seconds since 1970-01-01 00:00:00 UTC.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/string_functions#upper"><code dir="ltr" translate="no">        UPPER       </code></a></td>
<td>Formats alphabetic characters in a <code dir="ltr" translate="no">       STRING      </code> value as uppercase.<br />
<br />
Formats ASCII characters in a <code dir="ltr" translate="no">       BYTES      </code> value as uppercase.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#var_samp"><code dir="ltr" translate="no">        VAR_SAMP       </code></a></td>
<td>Computes the sample (unbiased) variance of the values.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/statistical_aggregate_functions#variance"><code dir="ltr" translate="no">        VARIANCE       </code></a></td>
<td>An alias of <code dir="ltr" translate="no">       VAR_SAMP      </code> .</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/compression-functions#zstd_compress"><code dir="ltr" translate="no">        ZSTD_COMPRESS       </code></a></td>
<td>Compresses <code dir="ltr" translate="no">       STRING      </code> or <code dir="ltr" translate="no">       BYTES      </code> input into <code dir="ltr" translate="no">       BYTES      </code> output using the <a href="https://en.wikipedia.org/wiki/Zstd">Zstandard (Zstd)</a> lossless data compression algorithm.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/compression-functions#zstd_decompress_to_bytes"><code dir="ltr" translate="no">        ZSTD_DECOMPRESS_TO_BYTES       </code></a></td>
<td>Decompresses <code dir="ltr" translate="no">       BYTES      </code> input into <code dir="ltr" translate="no">       BYTES      </code> output using the <a href="https://en.wikipedia.org/wiki/Zstd">Zstandard (Zstd)</a> lossless data compression algorithm.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/compression-functions#zstd_decompress_to_string"><code dir="ltr" translate="no">        ZSTD_DECOMPRESS_TO_STRING       </code></a></td>
<td>Decompress <code dir="ltr" translate="no">       BYTES      </code> input into <code dir="ltr" translate="no">       STRING      </code> output using the <a href="https://en.wikipedia.org/wiki/Zstd">Zstandard (Zstd)</a> lossless data compression algorithm.</td>
</tr>
</tbody>
</table>
