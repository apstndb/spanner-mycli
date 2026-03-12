GoogleSQL for Spanner supports the following date functions.

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
<td><a href="/spanner/docs/reference/standard-sql/date_functions#adddate"><code dir="ltr" translate="no">        ADDDATE       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       DATE_ADD      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#current_date"><code dir="ltr" translate="no">        CURRENT_DATE       </code></a></td>
<td>Returns the current date as a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date"><code dir="ltr" translate="no">        DATE       </code></a></td>
<td>Constructs a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_add"><code dir="ltr" translate="no">        DATE_ADD       </code></a></td>
<td>Adds a specified time interval to a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_diff"><code dir="ltr" translate="no">        DATE_DIFF       </code></a></td>
<td>Gets the number of unit boundaries between two <code dir="ltr" translate="no">       DATE      </code> values at a particular time granularity.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_from_unix_date"><code dir="ltr" translate="no">        DATE_FROM_UNIX_DATE       </code></a></td>
<td>Interprets an <code dir="ltr" translate="no">       INT64      </code> expression as the number of days since 1970-01-01.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_sub"><code dir="ltr" translate="no">        DATE_SUB       </code></a></td>
<td>Subtracts a specified time interval from a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#date_trunc"><code dir="ltr" translate="no">        DATE_TRUNC       </code></a></td>
<td>Truncates a <code dir="ltr" translate="no">       DATE      </code> value at a particular granularity.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#extract"><code dir="ltr" translate="no">        EXTRACT       </code></a></td>
<td>Extracts part of a date from a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#format_date"><code dir="ltr" translate="no">        FORMAT_DATE       </code></a></td>
<td>Formats a <code dir="ltr" translate="no">       DATE      </code> value according to a specified format string.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/array_functions#generate_date_array"><code dir="ltr" translate="no">        GENERATE_DATE_ARRAY       </code></a></td>
<td>Generates an array of dates in a range.<br />
For more information, see <a href="/spanner/docs/reference/standard-sql/array_functions">Array functions</a> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#parse_date"><code dir="ltr" translate="no">        PARSE_DATE       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       DATE      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#subdate"><code dir="ltr" translate="no">        SUBDATE       </code></a></td>
<td>Alias for <code dir="ltr" translate="no">       DATE_SUB      </code> .</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/date_functions#unix_date"><code dir="ltr" translate="no">        UNIX_DATE       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       DATE      </code> value to the number of days since 1970-01-01.</td>
</tr>
</tbody>
</table>

## `     ADDDATE    `

``` text
ADDDATE(date_expression, INTERVAL int64_expression date_part)
```

Alias for [`  DATE_ADD  `](#date_add) .

## `     CURRENT_DATE    `

``` text
CURRENT_DATE()
```

``` text
CURRENT_DATE(time_zone_expression)
```

``` text
CURRENT_DATE
```

**Description**

Returns the current date as a `  DATE  ` object. Parentheses are optional when called with no arguments.

This function supports the following arguments:

  - `  time_zone_expression  ` : A `  STRING  ` expression that represents a [time zone](/spanner/docs/reference/standard-sql/timestamp_functions#timezone_definitions) . If no time zone is specified, the default time zone, America/Los\_Angeles, is used. If this expression is used and it evaluates to `  NULL  ` , this function returns `  NULL  ` .

The current date value is set at the start of the query statement that contains this function. All invocations of `  CURRENT_DATE()  ` within a query statement yield the same value.

**Return Data Type**

`  DATE  `

**Examples**

The following query produces the current date in the default time zone:

``` text
SELECT CURRENT_DATE() AS the_date;

/*--------------+
 | the_date     |
 +--------------+
 | 2016-12-25   |
 +--------------*/
```

The following queries produce the current date in a specified time zone:

``` text
SELECT CURRENT_DATE('America/Los_Angeles') AS the_date;

/*--------------+
 | the_date     |
 +--------------+
 | 2016-12-25   |
 +--------------*/
```

``` text
SELECT CURRENT_DATE('-08') AS the_date;

/*--------------+
 | the_date     |
 +--------------+
 | 2016-12-25   |
 +--------------*/
```

The following query produces the current date in the default time zone. Parentheses aren't needed if the function has no arguments.

``` text
SELECT CURRENT_DATE AS the_date;

/*--------------+
 | the_date     |
 +--------------+
 | 2016-12-25   |
 +--------------*/
```

## `     DATE    `

``` text
DATE(year, month, day)
```

``` text
DATE(timestamp_expression)
```

``` text
DATE(timestamp_expression, time_zone_expression)
```

**Description**

Constructs or extracts a date.

This function supports the following arguments:

  - `  year  ` : The `  INT64  ` value for year.
  - `  month  ` : The `  INT64  ` value for month.
  - `  day  ` : The `  INT64  ` value for day.
  - `  timestamp_expression  ` : A `  TIMESTAMP  ` expression that contains the date.
  - `  time_zone_expression  ` : A `  STRING  ` expression that represents a [time zone](/spanner/docs/reference/standard-sql/timestamp_functions#timezone_definitions) . If no time zone is specified with `  timestamp_expression  ` , the default time zone, America/Los\_Angeles, is used.

**Return Data Type**

`  DATE  `

**Example**

``` text
SELECT
  DATE(2016, 12, 25) AS date_ymd,
  DATE(TIMESTAMP '2016-12-25 05:30:00+07', 'America/Los_Angeles') AS date_tstz;

/*------------+------------+
 | date_ymd   | date_tstz  |
 +------------+------------+
 | 2016-12-25 | 2016-12-24 |
 +------------+------------*/
```

## `     DATE_ADD    `

``` text
DATE_ADD(date_expression, INTERVAL int64_expression date_part)
```

**Description**

Adds a specified time interval to a DATE.

`  DATE_ADD  ` supports the following `  date_part  ` values:

  - `  DAY  `
  - `  WEEK  ` . Equivalent to 7 `  DAY  ` s.
  - `  MONTH  `
  - `  QUARTER  `
  - `  YEAR  `

Special handling is required for MONTH, QUARTER, and YEAR parts when the date is at (or near) the last day of the month. If the resulting month has fewer days than the original date's day, then the resulting date is the last date of that month.

**Return Data Type**

DATE

**Example**

``` text
SELECT DATE_ADD(DATE '2008-12-25', INTERVAL 5 DAY) AS five_days_later;

/*--------------------+
 | five_days_later    |
 +--------------------+
 | 2008-12-30         |
 +--------------------*/
```

## `     DATE_DIFF    `

``` text
DATE_DIFF(end_date, start_date, granularity)
```

**Description**

Gets the number of unit boundaries between two `  DATE  ` values ( `  end_date  ` - `  start_date  ` ) at a particular time granularity.

**Definitions**

  - `  start_date  ` : The starting `  DATE  ` value.

  - `  end_date  ` : The ending `  DATE  ` value.

  - `  granularity  ` : The date part that represents the granularity. This can be:
    
      - `  DAY  `
      - `  WEEK  ` This date part begins on Sunday.
      - `  ISOWEEK  ` : Uses [ISO 8601 week](https://en.wikipedia.org/wiki/ISO_week_date) boundaries. ISO weeks begin on Monday.
      - `  MONTH  `
      - `  QUARTER  `
      - `  YEAR  `
      - `  ISOYEAR  ` : Uses the [ISO 8601](https://en.wikipedia.org/wiki/ISO_8601) week-numbering year boundary. The ISO year boundary is the Monday of the first week whose Thursday belongs to the corresponding Gregorian calendar year.

**Details**

If `  end_date  ` is earlier than `  start_date  ` , the output is negative.

**Return Data Type**

`  INT64  `

**Example**

``` text
SELECT DATE_DIFF(DATE '2010-07-07', DATE '2008-12-25', DAY) AS days_diff;

/*-----------+
 | days_diff |
 +-----------+
 | 559       |
 +-----------*/
```

``` text
SELECT
  DATE_DIFF(DATE '2017-10-15', DATE '2017-10-14', DAY) AS days_diff,
  DATE_DIFF(DATE '2017-10-15', DATE '2017-10-14', WEEK) AS weeks_diff;

/*-----------+------------+
 | days_diff | weeks_diff |
 +-----------+------------+
 | 1         | 1          |
 +-----------+------------*/
```

The example above shows the result of `  DATE_DIFF  ` for two days in succession. `  DATE_DIFF  ` with the date part `  WEEK  ` returns 1 because `  DATE_DIFF  ` counts the number of date part boundaries in this range of dates. Each `  WEEK  ` begins on Sunday, so there is one date part boundary between Saturday, 2017-10-14 and Sunday, 2017-10-15.

The following example shows the result of `  DATE_DIFF  ` for two dates in different years. `  DATE_DIFF  ` with the date part `  YEAR  ` returns 3 because it counts the number of Gregorian calendar year boundaries between the two dates. `  DATE_DIFF  ` with the date part `  ISOYEAR  ` returns 2 because the second date belongs to the ISO year 2015. The first Thursday of the 2015 calendar year was 2015-01-01, so the ISO year 2015 begins on the preceding Monday, 2014-12-29.

``` text
SELECT
  DATE_DIFF('2017-12-30', '2014-12-30', YEAR) AS year_diff,
  DATE_DIFF('2017-12-30', '2014-12-30', ISOYEAR) AS isoyear_diff;

/*-----------+--------------+
 | year_diff | isoyear_diff |
 +-----------+--------------+
 | 3         | 2            |
 +-----------+--------------*/
```

The following example shows the result of `  DATE_DIFF  ` for two days in succession. The first date falls on a Monday and the second date falls on a Sunday. `  DATE_DIFF  ` with the date part `  WEEK  ` returns 0 because this date part uses weeks that begin on Sunday. `  DATE_DIFF  ` with the date part `  ISOWEEK  ` returns 1 because ISO weeks begin on Monday.

``` text
SELECT
  DATE_DIFF('2017-12-18', '2017-12-17', WEEK) AS week_diff,
  DATE_DIFF('2017-12-18', '2017-12-17', ISOWEEK) AS isoweek_diff;

/*-----------+--------------+
 | week_diff | isoweek_diff |
 +-----------+--------------+
 | 0         | 1            |
 +-----------+--------------*/
```

## `     DATE_FROM_UNIX_DATE    `

``` text
DATE_FROM_UNIX_DATE(int64_expression)
```

**Description**

Interprets `  int64_expression  ` as the number of days since 1970-01-01.

**Return Data Type**

DATE

**Example**

``` text
SELECT DATE_FROM_UNIX_DATE(14238) AS date_from_epoch;

/*-----------------+
 | date_from_epoch |
 +-----------------+
 | 2008-12-25      |
 +-----------------+*/
```

## `     DATE_SUB    `

``` text
DATE_SUB(date_expression, INTERVAL int64_expression date_part)
```

**Description**

Subtracts a specified time interval from a DATE.

`  DATE_SUB  ` supports the following `  date_part  ` values:

  - `  DAY  `
  - `  WEEK  ` . Equivalent to 7 `  DAY  ` s.
  - `  MONTH  `
  - `  QUARTER  `
  - `  YEAR  `

Special handling is required for MONTH, QUARTER, and YEAR parts when the date is at (or near) the last day of the month. If the resulting month has fewer days than the original date's day, then the resulting date is the last date of that month.

**Return Data Type**

DATE

**Example**

``` text
SELECT DATE_SUB(DATE '2008-12-25', INTERVAL 5 DAY) AS five_days_ago;

/*---------------+
 | five_days_ago |
 +---------------+
 | 2008-12-20    |
 +---------------*/
```

## `     DATE_TRUNC    `

``` text
DATE_TRUNC(date_value, date_granularity)
```

**Description**

Truncates a `  DATE  ` value at a particular granularity.

**Definitions**

  - `  date_value  ` : A `  DATE  ` value to truncate.
  - `  date_granularity  ` : The truncation granularity for a `  DATE  ` value. [Date granularities](#date_trunc_granularity_date) can be used.

**Date granularity definitions**

  - `  DAY  ` : The day in the Gregorian calendar year that contains the value to truncate.

  - `  WEEK  ` : The first day in the week that contains the value to truncate. Weeks begin on Sundays. `  WEEK  ` is equivalent to `  WEEK(SUNDAY)  ` .

  - `  ISOWEEK  ` : The first day in the [ISO 8601 week](https://en.wikipedia.org/wiki/ISO_week_date) that contains the value to truncate. The ISO week begins on Monday. The first ISO week of each ISO year contains the first Thursday of the corresponding Gregorian calendar year.

  - `  MONTH  ` : The first day in the month that contains the value to truncate.

  - `  QUARTER  ` : The first day in the quarter that contains the value to truncate.

  - `  YEAR  ` : The first day in the year that contains the value to truncate.

  - `  ISOYEAR  ` : The first day in the [ISO 8601](https://en.wikipedia.org/wiki/ISO_8601) week-numbering year that contains the value to truncate. The ISO year is the Monday of the first week where Thursday belongs to the corresponding Gregorian calendar year.

**Details**

The resulting value is always rounded to the beginning of `  granularity  ` .

**Return Data Type**

`  DATE  `

**Examples**

``` text
SELECT DATE_TRUNC(DATE '2008-12-25', MONTH) AS month;

/*------------+
 | month      |
 +------------+
 | 2008-12-01 |
 +------------*/
```

In the following example, the original `  date_expression  ` is in the Gregorian calendar year 2015. However, `  DATE_TRUNC  ` with the `  ISOYEAR  ` date part truncates the `  date_expression  ` to the beginning of the ISO year, not the Gregorian calendar year. The first Thursday of the 2015 calendar year was 2015-01-01, so the ISO year 2015 begins on the preceding Monday, 2014-12-29. Therefore the ISO year boundary preceding the `  date_expression  ` 2015-06-15 is 2014-12-29.

``` text
SELECT
  DATE_TRUNC('2015-06-15', ISOYEAR) AS isoyear_boundary,
  EXTRACT(ISOYEAR FROM DATE '2015-06-15') AS isoyear_number;

/*------------------+----------------+
 | isoyear_boundary | isoyear_number |
 +------------------+----------------+
 | 2014-12-29       | 2015           |
 +------------------+----------------*/
```

## `     EXTRACT    `

``` text
EXTRACT(part FROM date_expression)
```

**Description**

Returns the value corresponding to the specified date part. The `  part  ` must be one of:

  - `  DAYOFWEEK  ` : Returns values in the range \[1,7\] with Sunday as the first day of the week.
  - `  DAY  `
  - `  DAYOFYEAR  `
  - `  WEEK  ` : Returns the week number of the date in the range \[0, 53\]. Weeks begin with Sunday, and dates prior to the first Sunday of the year are in week 0.
  - `  ISOWEEK  ` : Returns the [ISO 8601 week](https://en.wikipedia.org/wiki/ISO_week_date) number of the `  date_expression  ` . `  ISOWEEK  ` s begin on Monday. Return values are in the range \[1, 53\]. The first `  ISOWEEK  ` of each ISO year begins on the Monday before the first Thursday of the Gregorian calendar year.
  - `  MONTH  `
  - `  QUARTER  ` : Returns values in the range \[1,4\].
  - `  YEAR  `
  - `  ISOYEAR  ` : Returns the [ISO 8601](https://en.wikipedia.org/wiki/ISO_8601) week-numbering year, which is the Gregorian calendar year containing the Thursday of the week to which `  date_expression  ` belongs.

**Return Data Type**

INT64

**Examples**

In the following example, `  EXTRACT  ` returns a value corresponding to the `  DAY  ` date part.

``` text
SELECT EXTRACT(DAY FROM DATE '2013-12-25') AS the_day;

/*---------+
 | the_day |
 +---------+
 | 25      |
 +---------*/
```

In the following example, `  EXTRACT  ` returns values corresponding to different date parts from a column of dates near the end of the year.

``` text
SELECT
  date,
  EXTRACT(ISOYEAR FROM date) AS isoyear,
  EXTRACT(ISOWEEK FROM date) AS isoweek,
  EXTRACT(YEAR FROM date) AS year,
  EXTRACT(WEEK FROM date) AS week
FROM UNNEST(GENERATE_DATE_ARRAY('2015-12-23', '2016-01-09')) AS date
ORDER BY date;

/*------------+---------+---------+------+------+
 | date       | isoyear | isoweek | year | week |
 +------------+---------+---------+------+------+
 | 2015-12-23 | 2015    | 52      | 2015 | 51   |
 | 2015-12-24 | 2015    | 52      | 2015 | 51   |
 | 2015-12-25 | 2015    | 52      | 2015 | 51   |
 | 2015-12-26 | 2015    | 52      | 2015 | 51   |
 | 2015-12-27 | 2015    | 52      | 2015 | 52   |
 | 2015-12-28 | 2015    | 53      | 2015 | 52   |
 | 2015-12-29 | 2015    | 53      | 2015 | 52   |
 | 2015-12-30 | 2015    | 53      | 2015 | 52   |
 | 2015-12-31 | 2015    | 53      | 2015 | 52   |
 | 2016-01-01 | 2015    | 53      | 2016 | 0    |
 | 2016-01-02 | 2015    | 53      | 2016 | 0    |
 | 2016-01-03 | 2015    | 53      | 2016 | 1    |
 | 2016-01-04 | 2016    | 1       | 2016 | 1    |
 | 2016-01-05 | 2016    | 1       | 2016 | 1    |
 | 2016-01-06 | 2016    | 1       | 2016 | 1    |
 | 2016-01-07 | 2016    | 1       | 2016 | 1    |
 | 2016-01-08 | 2016    | 1       | 2016 | 1    |
 | 2016-01-09 | 2016    | 1       | 2016 | 1    |
 +------------+---------+---------+------+------*/
```

## `     FORMAT_DATE    `

``` text
FORMAT_DATE(format_string, date_expr)
```

**Description**

Formats a `  DATE  ` value according to a specified format string.

**Definitions**

  - `  format_string  ` : A `  STRING  ` value that contains the [format elements](/spanner/docs/reference/standard-sql/format-elements#format_elements_date_time) to use with `  date_expr  ` .
  - `  date_expr  ` : A `  DATE  ` value that represents the date to format.

**Return Data Type**

`  STRING  `

**Examples**

``` text
SELECT FORMAT_DATE('%x', DATE '2008-12-25') AS US_format;

/*------------+
 | US_format  |
 +------------+
 | 12/25/08   |
 +------------*/
```

``` text
SELECT FORMAT_DATE('%b-%d-%Y', DATE '2008-12-25') AS formatted;

/*-------------+
 | formatted   |
 +-------------+
 | Dec-25-2008 |
 +-------------*/
```

``` text
SELECT FORMAT_DATE('%b %Y', DATE '2008-12-25') AS formatted;

/*-------------+
 | formatted   |
 +-------------+
 | Dec 2008    |
 +-------------*/
```

## `     PARSE_DATE    `

``` text
PARSE_DATE(format_string, date_string)
```

**Description**

Converts a `  STRING  ` value to a `  DATE  ` value.

**Definitions**

  - `  format_string  ` : A `  STRING  ` value that contains the [format elements](/spanner/docs/reference/standard-sql/format-elements#format_elements_date_time) to use with `  date_string  ` .
  - `  date_string  ` : A `  STRING  ` value that represents the date to parse.

**Details**

Each element in `  date_string  ` must have a corresponding element in `  format_string  ` . The location of each element in `  format_string  ` must match the location of each element in `  date_string  ` .

``` text
-- This works because elements on both sides match.
SELECT PARSE_DATE('%A %b %e %Y', 'Thursday Dec 25 2008');

-- This produces an error because the year element is in different locations.
SELECT PARSE_DATE('%Y %A %b %e', 'Thursday Dec 25 2008');

-- This produces an error because one of the year elements is missing.
SELECT PARSE_DATE('%A %b %e', 'Thursday Dec 25 2008');

-- This works because %F can find all matching elements in date_string.
SELECT PARSE_DATE('%F', '2000-12-30');
```

The format string fully supports most format elements except for `  %g  ` , `  %G  ` , `  %j  ` , `  %u  ` , `  %U  ` , `  %V  ` , `  %w  ` , and `  %W  ` .

The following additional considerations apply when using the `  PARSE_DATE  ` function:

  - Unspecified fields. Any unspecified field is initialized from `  1970-01-01  ` .
  - Case insensitivity. Names, such as `  Monday  ` , `  February  ` , and so on, are case insensitive.
  - Whitespace. One or more consecutive white spaces in the format string matches zero or more consecutive white spaces in the date string. In addition, leading and trailing white spaces in the date string are always allowed, even if they aren't in the format string.
  - Format precedence. When two (or more) format elements have overlapping information (for example both `  %F  ` and `  %Y  ` affect the year), the last one generally overrides any earlier ones.

**Return Data Type**

`  DATE  `

**Examples**

This example converts a `  MM/DD/YY  ` formatted string to a `  DATE  ` object:

``` text
SELECT PARSE_DATE('%x', '12/25/08') AS parsed;

/*------------+
 | parsed     |
 +------------+
 | 2008-12-25 |
 +------------*/
```

This example converts a `  YYYYMMDD  ` formatted string to a `  DATE  ` object:

``` text
SELECT PARSE_DATE('%Y%m%d', '20081225') AS parsed;

/*------------+
 | parsed     |
 +------------+
 | 2008-12-25 |
 +------------*/
```

## `     SUBDATE    `

``` text
SUBDATE(date_expression, INTERVAL int64_expression date_part)
```

Alias for [`  DATE_SUB  `](#date_sub) .

## `     UNIX_DATE    `

``` text
UNIX_DATE(date_expression)
```

**Description**

Returns the number of days since `  1970-01-01  ` .

**Return Data Type**

INT64

**Example**

``` text
SELECT UNIX_DATE(DATE '2008-12-25') AS days_from_epoch;

/*-----------------+
 | days_from_epoch |
 +-----------------+
 | 14238           |
 +-----------------*/
```
