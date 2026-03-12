GoogleSQL for Spanner supports the following timestamp functions.

IMPORTANT: Before working with these functions, you need to understand the difference between the formats in which timestamps are stored and displayed, and how time zones are used for the conversion between these formats. To learn more, see [How time zones work with timestamp functions](#timezone_definitions) .

NOTE: These functions return a runtime error if overflow occurs; result values are bounded by the defined [`  DATE  ` range](/spanner/docs/reference/standard-sql/data-types#date_type) and [`  TIMESTAMP  ` range](/spanner/docs/reference/standard-sql/data-types#timestamp_type) .

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
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#current_timestamp"><code dir="ltr" translate="no">        CURRENT_TIMESTAMP       </code></a></td>
<td>Returns the current date and time as a <code dir="ltr" translate="no">       TIMESTAMP      </code> object.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#extract"><code dir="ltr" translate="no">        EXTRACT       </code></a></td>
<td>Extracts part of a <code dir="ltr" translate="no">       TIMESTAMP      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#format_timestamp"><code dir="ltr" translate="no">        FORMAT_TIMESTAMP       </code></a></td>
<td>Formats a <code dir="ltr" translate="no">       TIMESTAMP      </code> value according to the specified format string.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#parse_timestamp"><code dir="ltr" translate="no">        PARSE_TIMESTAMP       </code></a></td>
<td>Converts a <code dir="ltr" translate="no">       STRING      </code> value to a <code dir="ltr" translate="no">       TIMESTAMP      </code> value.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#pending_commit_timestamp"><code dir="ltr" translate="no">        PENDING_COMMIT_TIMESTAMP       </code></a></td>
<td>Write a pending commit timestamp.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/timestamp_functions#string"><code dir="ltr" translate="no">        STRING       </code> (Timestamp)</a></td>
<td>Converts a <code dir="ltr" translate="no">       TIMESTAMP      </code> value to a <code dir="ltr" translate="no">       STRING      </code> value.</td>
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
</tbody>
</table>

## `     CURRENT_TIMESTAMP    `

``` text
CURRENT_TIMESTAMP()
```

``` text
CURRENT_TIMESTAMP
```

**Description**

Returns the current date and time as a timestamp object. The timestamp is continuous, non-ambiguous, has exactly 60 seconds per minute and doesn't repeat values over the leap second. Parentheses are optional.

This function handles leap seconds by smearing them across a window of 20 hours around the inserted leap second.

The current timestamp value is set at the start of the query statement that contains this function. All invocations of `  CURRENT_TIMESTAMP()  ` within a query statement yield the same value.

**Supported Input Types**

Not applicable

**Result Data Type**

`  TIMESTAMP  `

**Examples**

``` text
SELECT CURRENT_TIMESTAMP() AS now;

/*--------------------------------+
 | now                            |
 +--------------------------------+
 | 2020-06-02T23:58:40.347847393Z |
 +--------------------------------*/
```

## `     EXTRACT    `

``` text
EXTRACT(part FROM timestamp_expression [AT TIME ZONE time_zone])
```

**Description**

Returns a value that corresponds to the specified `  part  ` from a supplied `  timestamp_expression  ` . This function supports an optional `  time_zone  ` parameter. See [Time zone definitions](#timezone_definitions) for information on how to specify a time zone.

Allowed `  part  ` values are:

  - `  NANOSECOND  `
  - `  MICROSECOND  `
  - `  MILLISECOND  `
  - `  SECOND  `
  - `  MINUTE  `
  - `  HOUR  `
  - `  DAYOFWEEK  ` : Returns values in the range \[1,7\] with Sunday as the first day of of the week.
  - `  DAY  `
  - `  DAYOFYEAR  `
  - `  WEEK  ` : Returns the week number of the date in the range \[0, 53\]. Weeks begin with Sunday, and dates prior to the first Sunday of the year are in week 0.
  - `  ISOWEEK  ` : Returns the [ISO 8601 week](https://en.wikipedia.org/wiki/ISO_week_date) number of the `  datetime_expression  ` . `  ISOWEEK  ` s begin on Monday. Return values are in the range \[1, 53\]. The first `  ISOWEEK  ` of each ISO year begins on the Monday before the first Thursday of the Gregorian calendar year.
  - `  MONTH  `
  - `  QUARTER  `
  - `  YEAR  `
  - `  ISOYEAR  ` : Returns the [ISO 8601](https://en.wikipedia.org/wiki/ISO_8601) week-numbering year, which is the Gregorian calendar year containing the Thursday of the week to which `  date_expression  ` belongs.
  - `  DATE  `

Returned values truncate lower order time periods. For example, when extracting seconds, `  EXTRACT  ` truncates the millisecond and microsecond values.

**Return Data Type**

`  INT64  ` , except in the following cases:

  - If `  part  ` is `  DATE  ` , the function returns a `  DATE  ` object.

**Examples**

In the following example, `  EXTRACT  ` returns a value corresponding to the `  DAY  ` time part.

``` text
SELECT
  EXTRACT(
    DAY
    FROM TIMESTAMP('2008-12-25 05:30:00+00') AT TIME ZONE 'UTC')
    AS the_day_utc,
  EXTRACT(
    DAY
    FROM TIMESTAMP('2008-12-25 05:30:00+00') AT TIME ZONE 'America/Los_Angeles')
    AS the_day_california

/*-------------+--------------------+
 | the_day_utc | the_day_california |
 +-------------+--------------------+
 | 25          | 24                 |
 +-------------+--------------------*/
```

In the following examples, `  EXTRACT  ` returns values corresponding to different time parts from a column of type `  TIMESTAMP  ` .

``` text
SELECT
  EXTRACT(ISOYEAR FROM TIMESTAMP("2005-01-03 12:34:56+00")) AS isoyear,
  EXTRACT(ISOWEEK FROM TIMESTAMP("2005-01-03 12:34:56+00")) AS isoweek,
  EXTRACT(YEAR FROM TIMESTAMP("2005-01-03 12:34:56+00")) AS year,
  EXTRACT(WEEK FROM TIMESTAMP("2005-01-03 12:34:56+00")) AS week

-- Display of results may differ, depending upon the environment and
-- time zone where this query was executed.
/*---------+---------+------+------+
 | isoyear | isoweek | year | week |
 +---------+---------+------+------+
 | 2005    | 1       | 2005 | 1    |
 +---------+---------+------+------*/
```

``` text
SELECT
  TIMESTAMP("2007-12-31 12:00:00+00") AS timestamp_value,
  EXTRACT(ISOYEAR FROM TIMESTAMP("2007-12-31 12:00:00+00")) AS isoyear,
  EXTRACT(ISOWEEK FROM TIMESTAMP("2007-12-31 12:00:00+00")) AS isoweek,
  EXTRACT(YEAR FROM TIMESTAMP("2007-12-31 12:00:00+00")) AS year,
  EXTRACT(WEEK FROM TIMESTAMP("2007-12-31 12:00:00+00")) AS week

-- Display of results may differ, depending upon the environment and time zone
-- where this query was executed.
/*---------+---------+------+------+
 | isoyear | isoweek | year | week |
 +---------+---------+------+------+
 | 2008    | 1       | 2007 | 52    |
 +---------+---------+------+------*/
```

``` text
SELECT
  TIMESTAMP("2009-01-01 12:00:00+00") AS timestamp_value,
  EXTRACT(ISOYEAR FROM TIMESTAMP("2009-01-01 12:00:00+00")) AS isoyear,
  EXTRACT(ISOWEEK FROM TIMESTAMP("2009-01-01 12:00:00+00")) AS isoweek,
  EXTRACT(YEAR FROM TIMESTAMP("2009-01-01 12:00:00+00")) AS year,
  EXTRACT(WEEK FROM TIMESTAMP("2009-01-01 12:00:00+00")) AS week

-- Display of results may differ, depending upon the environment and time zone
-- where this query was executed.
/*---------+---------+------+------+
 | isoyear | isoweek | year | week |
 +---------+---------+------+------+
 | 2009    | 1       | 2009 | 0    |
 +---------+---------+------+------*/
```

``` text
SELECT
  TIMESTAMP("2009-12-31 12:00:00+00") AS timestamp_value,
  EXTRACT(ISOYEAR FROM TIMESTAMP("2009-12-31 12:00:00+00")) AS isoyear,
  EXTRACT(ISOWEEK FROM TIMESTAMP("2009-12-31 12:00:00+00")) AS isoweek,
  EXTRACT(YEAR FROM TIMESTAMP("2009-12-31 12:00:00+00")) AS year,
  EXTRACT(WEEK FROM TIMESTAMP("2009-12-31 12:00:00+00")) AS week

-- Display of results may differ, depending upon the environment and time zone
-- where this query was executed.
/*---------+---------+------+------+
 | isoyear | isoweek | year | week |
 +---------+---------+------+------+
 | 2009    | 53      | 2009 | 52   |
 +---------+---------+------+------*/
```

``` text
SELECT
  TIMESTAMP("2017-01-02 12:00:00+00") AS timestamp_value,
  EXTRACT(ISOYEAR FROM TIMESTAMP("2017-01-02 12:00:00+00")) AS isoyear,
  EXTRACT(ISOWEEK FROM TIMESTAMP("2017-01-02 12:00:00+00")) AS isoweek,
  EXTRACT(YEAR FROM TIMESTAMP("2017-01-02 12:00:00+00")) AS year,
  EXTRACT(WEEK FROM TIMESTAMP("2017-01-02 12:00:00+00")) AS week

-- Display of results may differ, depending upon the environment and time zone
-- where this query was executed.
/*---------+---------+------+------+
 | isoyear | isoweek | year | week |
 +---------+---------+------+------+
 | 2017    | 1       | 2017 | 1    |
 +---------+---------+------+------*/
```

``` text
SELECT
  TIMESTAMP("2017-05-26 12:00:00+00") AS timestamp_value,
  EXTRACT(ISOYEAR FROM TIMESTAMP("2017-05-26 12:00:00+00")) AS isoyear,
  EXTRACT(ISOWEEK FROM TIMESTAMP("2017-05-26 12:00:00+00")) AS isoweek,
  EXTRACT(YEAR FROM TIMESTAMP("2017-05-26 12:00:00+00")) AS year,
  EXTRACT(WEEK FROM TIMESTAMP("2017-05-26 12:00:00+00")) AS week

-- Display of results may differ, depending upon the environment and time zone
-- where this query was executed.
/*---------+---------+------+------+
 | isoyear | isoweek | year | week |
 +---------+---------+------+------+
 | 2017    | 21      | 2017 | 21   |
 +---------+---------+------+------*/
```

## `     FORMAT_TIMESTAMP    `

``` text
FORMAT_TIMESTAMP(format_string, timestamp_expr[, time_zone])
```

**Description**

Formats a `  TIMESTAMP  ` value according to the specified format string.

**Definitions**

  - `  format_string  ` : A `  STRING  ` value that contains the [format elements](/spanner/docs/reference/standard-sql/format-elements#format_elements_date_time) to use with `  timestamp_expr  ` .
  - `  timestamp_expr  ` : A `  TIMESTAMP  ` value that represents the timestamp to format.
  - `  time_zone  ` : A `  STRING  ` value that represents a time zone. For more information about how to use a time zone with a timestamp, see [Time zone definitions](#timezone_definitions) .

**Return Data Type**

`  STRING  `

**Examples**

``` text
SELECT FORMAT_TIMESTAMP("%c", TIMESTAMP "2050-12-25 15:30:55+00", "UTC")
  AS formatted;

/*--------------------------+
 | formatted                |
 +--------------------------+
 | Sun Dec 25 15:30:55 2050 |
 +--------------------------*/
```

``` text
SELECT FORMAT_TIMESTAMP("%b-%d-%Y", TIMESTAMP "2050-12-25 15:30:55+00")
  AS formatted;

/*-------------+
 | formatted   |
 +-------------+
 | Dec-25-2050 |
 +-------------*/
```

``` text
SELECT FORMAT_TIMESTAMP("%b %Y", TIMESTAMP "2050-12-25 15:30:55+00")
  AS formatted;

/*-------------+
 | formatted   |
 +-------------+
 | Dec 2050    |
 +-------------*/
```

``` text
SELECT FORMAT_TIMESTAMP("%Y-%m-%dT%H:%M:%S%Z", TIMESTAMP "2050-12-25 15:30:55", "UTC")
  AS formatted;

/*+-----------------------+
 |       formatted        |
 +------------------------+
 | 2050-12-25T15:30:55UTC |
 +------------------------*/
```

## `     PARSE_TIMESTAMP    `

``` text
PARSE_TIMESTAMP(format_string, timestamp_string[, time_zone])
```

**Description**

Converts a `  STRING  ` value to a `  TIMESTAMP  ` value.

**Definitions**

  - `  format_string  ` : A `  STRING  ` value that contains the [format elements](/spanner/docs/reference/standard-sql/format-elements#format_elements_date_time) to use with `  timestamp_string  ` .
  - `  timestamp_string  ` : A `  STRING  ` value that represents the timestamp to parse.
  - `  time_zone  ` : A `  STRING  ` value that represents a time zone. For more information about how to use a time zone with a timestamp, see [Time zone definitions](#timezone_definitions) .

**Details**

Each element in `  timestamp_string  ` must have a corresponding element in `  format_string  ` . The location of each element in `  format_string  ` must match the location of each element in `  timestamp_string  ` .

``` text
-- This works because elements on both sides match.
SELECT PARSE_TIMESTAMP("%a %b %e %I:%M:%S %Y", "Thu Dec 25 07:30:00 2008");

-- This produces an error because the year element is in different locations.
SELECT PARSE_TIMESTAMP("%a %b %e %Y %I:%M:%S", "Thu Dec 25 07:30:00 2008");

-- This produces an error because one of the year elements is missing.
SELECT PARSE_TIMESTAMP("%a %b %e %I:%M:%S", "Thu Dec 25 07:30:00 2008");

-- This works because %c can find all matching elements in timestamp_string.
SELECT PARSE_TIMESTAMP("%c", "Thu Dec 25 07:30:00 2008");
```

The format string fully supports most format elements, except for `  %g  ` , `  %G  ` , `  %j  ` , `  %P  ` , `  %u  ` , `  %U  ` , `  %V  ` , `  %w  ` , and `  %W  ` .

The following additional considerations apply when using the `  PARSE_TIMESTAMP  ` function:

  - Unspecified fields. Any unspecified field is initialized from `  1970-01-01 00:00:00.0  ` . This initialization value uses the time zone specified by the function's time zone argument, if present. If not, the initialization value uses the default time zone, America/Los\_Angeles. For instance, if the year is unspecified then it defaults to `  1970  ` , and so on.
  - Case insensitivity. Names, such as `  Monday  ` , `  February  ` , and so on, are case insensitive.
  - Whitespace. One or more consecutive white spaces in the format string matches zero or more consecutive white spaces in the timestamp string. In addition, leading and trailing white spaces in the timestamp string are always allowed, even if they aren't in the format string.
  - Format precedence. When two (or more) format elements have overlapping information (for example both `  %F  ` and `  %Y  ` affect the year), the last one generally overrides any earlier ones, with some exceptions (see the descriptions of `  %s  ` , `  %C  ` , and `  %y  ` ).
  - Format divergence. `  %p  ` can be used with `  am  ` , `  AM  ` , `  pm  ` , and `  PM  ` .

**Return Data Type**

`  TIMESTAMP  `

**Example**

``` text
SELECT PARSE_TIMESTAMP("%c", "Thu Dec 25 07:30:00 2008") AS parsed;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*------------------------+
 | parsed                 |
 +------------------------+
 | 2008-12-25T15:30:00Z   |
 +------------------------*/
```

## `     PENDING_COMMIT_TIMESTAMP    `

``` text
PENDING_COMMIT_TIMESTAMP()
```

**Description**

Use the `  PENDING_COMMIT_TIMESTAMP  ` function in a DML `  INSERT  ` or `  UPDATE  ` statement to write the pending commit timestamp, that is, the commit timestamp of the write when it commits, into a column of type `  TIMESTAMP  ` . You can also use this function as a default value and `  ON UPDATE  ` value, but only if you use the `  allow_commit_timestamp=true  ` column option with it.

selects the commit timestamp when the transaction commits. You can use the `  PENDING_COMMIT_TIMESTAMP  ` function as a value only when inserting or updating an appropriately typed column. It can't be used in a `  SELECT  ` statement, or as the input to any other scalar expression.

**Note:** After you call the `  PENDING_COMMIT_TIMESTAMP  ` function, the table and any derived index is unreadable to any future SQL statements in the transaction. Because of this, the change stream can't extract the previous value for the column that has a pending commit timestamp, if the column is modified again later in the same transaction. You must write commit timestamps as the last statement in a transaction to prevent the possibility of trying to read the table. If you try to read the table, then GoogleSQL produces an error.

**Return Data Type**

TIMESTAMP

**Example**

The following DML statement updates the `  LastUpdatedTime  ` column in the `  Singers  ` table with the commit timestamp.

``` text
UPDATE Performances SET LastUpdatedTime = PENDING_COMMIT_TIMESTAMP()
   WHERE SingerId=1 AND VenueId=2 AND EventDate="2015-10-21"
```

The following DML statement creates a table with the `  PENDING_COMMIT_TIMESTAMP  ` function used in the following ways:

  - As the default value for the `  CreatedTime  ` column.
  - As the default value for the `  LastUpdatedTime  ` column.
  - As the `  ON UPDATE  ` value for the `  LastUpdatedTime  ` column.

If no value is provided for the `  CreatedTime  ` or `  LastUpdatedTime  ` column in an `  INSERT  ` command, then this default value is used. If no value is provided for the `  LastUpdatedTime  ` column in an `  UPDATE  ` command, then this `  ON UPDATE  ` value is used.

``` text
CREATE TABLE Performances (
    SingerId        INT64 NOT NULL,
    VenueId         INT64 NOT NULL,
    EventDate       DATE,
    Revenue         INT64,
    CreatedTime     NOT NULL DEFAULT (PENDING_COMMIT_TIMESTAMP()) OPTIONS (allow_commit_timestamp=true)
    LastUpdatedTime  NOT NULL DEFAULT (PENDING_COMMIT_TIMESTAMP()) ON UPDATE (PENDING_COMMIT_TIMESTAMP())
      OPTIONS (allow_commit_timestamp=true)
) PRIMARY KEY (SingerId, VenueId, EventDate),
  INTERLEAVE IN PARENT Singers ON DELETE CASCADE
```

## `     STRING    `

``` text
STRING(timestamp_expression[, time_zone])
```

**Description**

Converts a timestamp to a string. Supports an optional parameter to specify a time zone. See [Time zone definitions](#timezone_definitions) for information on how to specify a time zone.

**Return Data Type**

`  STRING  `

**Example**

``` text
SELECT STRING(TIMESTAMP "2008-12-25 15:30:00+00", "UTC") AS string;

/*-------------------------------+
 | string                        |
 +-------------------------------+
 | 2008-12-25 15:30:00+00        |
 +-------------------------------*/
```

## `     TIMESTAMP    `

``` text
TIMESTAMP(string_expression[, time_zone])
TIMESTAMP(date_expression[, time_zone])
```

**Description**

  - `  string_expression[, time_zone]  ` : Converts a string to a timestamp. `  string_expression  ` must include a timestamp literal. If `  string_expression  ` includes a time zone in the timestamp literal, don't include an explicit `  time_zone  ` argument.
  - `  date_expression[, time_zone]  ` : Converts a date to a timestamp. The value returned is the earliest timestamp that falls within the given date.

This function supports an optional parameter to [specify a time zone](#timezone_definitions) . If no time zone is specified, the default time zone, America/Los\_Angeles, is used.

**Return Data Type**

`  TIMESTAMP  `

**Examples**

``` text
SELECT TIMESTAMP("2008-12-25 15:30:00+00") AS timestamp_str;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*----------------------+
 | timestamp_str        |
 +----------------------+
 | 2008-12-25T15:30:00Z |
 +----------------------*/
```

``` text
SELECT TIMESTAMP("2008-12-25 15:30:00", "America/Los_Angeles") AS timestamp_str;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*----------------------+
 | timestamp_str        |
 +----------------------+
 | 2008-12-25T23:30:00Z |
 +----------------------*/
```

``` text
SELECT TIMESTAMP("2008-12-25 15:30:00 UTC") AS timestamp_str;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*----------------------+
 | timestamp_str        |
 +----------------------+
 | 2008-12-25T15:30:00Z |
 +----------------------*/
```

``` text
SELECT TIMESTAMP(DATE "2008-12-25") AS timestamp_date;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*----------------------+
 | timestamp_date       |
 +----------------------+
 | 2008-12-25T08:00:00Z |
 +----------------------*/
```

## `     TIMESTAMP_ADD    `

``` text
TIMESTAMP_ADD(timestamp_expression, INTERVAL int64_expression date_part)
```

**Description**

Adds `  int64_expression  ` units of `  date_part  ` to the timestamp, independent of any time zone.

`  TIMESTAMP_ADD  ` supports the following values for `  date_part  ` :

  - `  NANOSECOND  `
  - `  MICROSECOND  `
  - `  MILLISECOND  `
  - `  SECOND  `
  - `  MINUTE  `
  - `  HOUR  ` . Equivalent to 60 `  MINUTE  ` parts.
  - `  DAY  ` . Equivalent to 24 `  HOUR  ` parts.

**Return Data Types**

`  TIMESTAMP  `

**Example**

``` text
SELECT
  TIMESTAMP("2008-12-25 15:30:00+00") AS original,
  TIMESTAMP_ADD(TIMESTAMP "2008-12-25 15:30:00+00", INTERVAL 10 MINUTE) AS later;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*------------------------+------------------------+
 | original               | later                  |
 +------------------------+------------------------+
 | 2008-12-25T15:30:00Z   | 2008-12-25T15:40:00Z   |
 +------------------------+------------------------*/
```

## `     TIMESTAMP_DIFF    `

``` text
TIMESTAMP_DIFF(end_timestamp, start_timestamp, granularity)
```

**Description**

Gets the number of unit boundaries between two `  TIMESTAMP  ` values ( `  end_timestamp  ` - `  start_timestamp  ` ) at a particular time granularity.

**Definitions**

  - `  start_timestamp  ` : The starting `  TIMESTAMP  ` value.

  - `  end_timestamp  ` : The ending `  TIMESTAMP  ` value.

  - `  granularity  ` : The timestamp part that represents the granularity. This can be:
    
      - `  NANOSECOND  `
      - `  MICROSECOND  `
      - `  MILLISECOND  `
      - `  SECOND  `
      - `  MINUTE  `
      - `  HOUR  ` . Equivalent to 60 `  MINUTE  ` s.
      - `  DAY  ` . Equivalent to 24 `  HOUR  ` s.

**Details**

If `  end_timestamp  ` is earlier than `  start_timestamp  ` , the output is negative. Produces an error if the computation overflows, such as if the difference in nanoseconds between the two `  TIMESTAMP  ` values overflows.

**Return Data Type**

`  INT64  `

**Example**

``` text
SELECT
  TIMESTAMP("2010-07-07 10:20:00+00") AS later_timestamp,
  TIMESTAMP("2008-12-25 15:30:00+00") AS earlier_timestamp,
  TIMESTAMP_DIFF(TIMESTAMP "2010-07-07 10:20:00+00", TIMESTAMP "2008-12-25 15:30:00+00", HOUR) AS hours;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*------------------------+------------------------+-------+
 | later_timestamp        | earlier_timestamp      | hours |
 +------------------------+------------------------+-------+
 | 2010-07-07T10:20:00Z   | 2008-12-25T15:30:00Z   | 13410 |
 +------------------------+------------------------+-------*/
```

In the following example, the first timestamp occurs before the second timestamp, resulting in a negative output.

``` text
SELECT TIMESTAMP_DIFF(TIMESTAMP "2018-08-14", TIMESTAMP "2018-10-14", DAY) AS negative_diff;

/*---------------+
 | negative_diff |
 +---------------+
 | -61           |
 +---------------*/
```

In this example, the result is 0 because only the number of whole specified `  HOUR  ` intervals are included.

``` text
SELECT TIMESTAMP_DIFF("2001-02-01 01:00:00", "2001-02-01 00:00:01", HOUR) AS diff;

/*---------------+
 | diff          |
 +---------------+
 | 0             |
 +---------------*/
```

## `     TIMESTAMP_MICROS    `

``` text
TIMESTAMP_MICROS(int64_expression)
```

**Description**

Interprets `  int64_expression  ` as the number of microseconds since 1970-01-01 00:00:00 UTC and returns a timestamp.

**Return Data Type**

`  TIMESTAMP  `

**Example**

``` text
SELECT TIMESTAMP_MICROS(1230219000000000) AS timestamp_value;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*------------------------+
 | timestamp_value        |
 +------------------------+
 | 2008-12-25T15:30:00Z   |
 +------------------------*/
```

## `     TIMESTAMP_MILLIS    `

``` text
TIMESTAMP_MILLIS(int64_expression)
```

**Description**

Interprets `  int64_expression  ` as the number of milliseconds since 1970-01-01 00:00:00 UTC and returns a timestamp.

**Return Data Type**

`  TIMESTAMP  `

**Example**

``` text
SELECT TIMESTAMP_MILLIS(1230219000000) AS timestamp_value;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*------------------------+
 | timestamp_value        |
 +------------------------+
 | 2008-12-25T15:30:00Z   |
 +------------------------*/
```

## `     TIMESTAMP_SECONDS    `

``` text
TIMESTAMP_SECONDS(int64_expression)
```

**Description**

Interprets `  int64_expression  ` as the number of seconds since 1970-01-01 00:00:00 UTC and returns a timestamp.

**Return Data Type**

`  TIMESTAMP  `

**Example**

``` text
SELECT TIMESTAMP_SECONDS(1230219000) AS timestamp_value;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*------------------------+
 | timestamp_value        |
 +------------------------+
 | 2008-12-25T15:30:00Z   |
 +------------------------*/
```

## `     TIMESTAMP_SUB    `

``` text
TIMESTAMP_SUB(timestamp_expression, INTERVAL int64_expression date_part)
```

**Description**

Subtracts `  int64_expression  ` units of `  date_part  ` from the timestamp, independent of any time zone.

`  TIMESTAMP_SUB  ` supports the following values for `  date_part  ` :

  - `  NANOSECOND  `
  - `  MICROSECOND  `
  - `  MILLISECOND  `
  - `  SECOND  `
  - `  MINUTE  `
  - `  HOUR  ` . Equivalent to 60 `  MINUTE  ` parts.
  - `  DAY  ` . Equivalent to 24 `  HOUR  ` parts.

**Return Data Type**

`  TIMESTAMP  `

**Example**

``` text
SELECT
  TIMESTAMP("2008-12-25 15:30:00+00") AS original,
  TIMESTAMP_SUB(TIMESTAMP "2008-12-25 15:30:00+00", INTERVAL 10 MINUTE) AS earlier;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*------------------------+------------------------+
 | original               | earlier                |
 +------------------------+------------------------+
 | 2008-12-25T15:30:00Z   | 2008-12-25T15:20:00Z   |
 +------------------------+------------------------*/
```

## `     TIMESTAMP_TRUNC    `

``` text
TIMESTAMP_TRUNC(timestamp_value, timestamp_granularity[, time_zone])
```

**Description**

Truncates a `  TIMESTAMP  ` value at a particular granularity.

**Definitions**

  - `  timestamp_value  ` : A `  TIMESTAMP  ` value to truncate.

  - `  timestamp_granularity  ` : The truncation granularity for a `  TIMESTAMP  ` value. [Date granularities](#timestamp_trunc_granularity_date) and [time granularities](#timestamp_trunc_granularity_time) can be used.

  - `  time_zone  ` : A time zone to use with the `  TIMESTAMP  ` value. [Time zone parts](#timestamp_time_zone_parts) can be used. Use this argument if you want to use a time zone other than the default time zone, America/Los\_Angeles, as part of the truncate operation.
    
    **Note:** When truncating a timestamp to `  MINUTE  ` or `  HOUR  ` parts, this function determines the civil time of the timestamp in the specified (or default) time zone and subtracts the minutes and seconds (when truncating to `  HOUR  ` ) or the seconds (when truncating to `  MINUTE  ` ) from that timestamp. While this provides intuitive results in most cases, the result is non-intuitive near daylight savings transitions that aren't hour-aligned.

**Date granularity definitions**

  - `  DAY  ` : The day in the Gregorian calendar year that contains the value to truncate.

  - `  WEEK  ` : The first day in the week that contains the value to truncate. Weeks begin on Sundays. `  WEEK  ` is equivalent to `  WEEK(SUNDAY)  ` .

  - `  ISOWEEK  ` : The first day in the [ISO 8601 week](https://en.wikipedia.org/wiki/ISO_week_date) that contains the value to truncate. The ISO week begins on Monday. The first ISO week of each ISO year contains the first Thursday of the corresponding Gregorian calendar year.

  - `  MONTH  ` : The first day in the month that contains the value to truncate.

  - `  QUARTER  ` : The first day in the quarter that contains the value to truncate.

  - `  YEAR  ` : The first day in the year that contains the value to truncate.

  - `  ISOYEAR  ` : The first day in the [ISO 8601](https://en.wikipedia.org/wiki/ISO_8601) week-numbering year that contains the value to truncate. The ISO year is the Monday of the first week where Thursday belongs to the corresponding Gregorian calendar year.

**Time granularity definitions**

  - `  NANOSECOND  ` : If used, nothing is truncated from the value.

  - `  MICROSECOND  ` : The nearest lesser than or equal microsecond.

  - `  MILLISECOND  ` : The nearest lesser than or equal millisecond.

  - `  SECOND  ` : The nearest lesser than or equal second.

  - `  MINUTE  ` : The nearest lesser than or equal minute.

  - `  HOUR  ` : The nearest lesser than or equal hour.

**Time zone part definitions**

  - `  MINUTE  `
  - `  HOUR  `
  - `  DAY  `
  - `  WEEK  `
  - `  ISOWEEK  `
  - `  MONTH  `
  - `  QUARTER  `
  - `  YEAR  `
  - `  ISOYEAR  `

**Details**

The resulting value is always rounded to the beginning of `  granularity  ` .

**Return Data Type**

`  TIMESTAMP  `

**Examples**

``` text
SELECT
  TIMESTAMP_TRUNC(TIMESTAMP "2008-12-25 15:30:00+00", DAY, "UTC") AS utc,
  TIMESTAMP_TRUNC(TIMESTAMP "2008-12-25 15:30:00+00", DAY, "America/Los_Angeles") AS la;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*------------------------+------------------------+
 | utc                    | la                     |
 +------------------------+------------------------+
 | 2008-12-25T00:00:00Z   | 2008-12-25T08:00:00Z   |
 +------------------------+------------------------*/
```

In the following example, the original `  timestamp_expression  ` is in the Gregorian calendar year 2015. However, `  TIMESTAMP_TRUNC  ` with the `  ISOYEAR  ` date part truncates the `  timestamp_expression  ` to the beginning of the ISO year, not the Gregorian calendar year. The first Thursday of the 2015 calendar year was 2015-01-01, so the ISO year 2015 begins on the preceding Monday, 2014-12-29. Therefore the ISO year boundary preceding the `  timestamp_expression  ` 2015-06-15 00:00:00+00 is 2014-12-29.

``` text
SELECT
  TIMESTAMP_TRUNC("2015-06-15 00:00:00+00", ISOYEAR) AS isoyear_boundary,
  EXTRACT(ISOYEAR FROM TIMESTAMP "2015-06-15 00:00:00+00") AS isoyear_number;

-- Display of results may differ, depending upon the environment and time zone where this query was executed.
/*------------------------+----------------+
 | parsed                 | isoyear_number |
 +------------------------+----------------+
 | 2014-12-29T08:00:00Z   | 2015           |
 +------------------------+----------------*/
```

## `     UNIX_MICROS    `

``` text
UNIX_MICROS(timestamp_expression)
```

**Description**

Returns the number of microseconds since `  1970-01-01 00:00:00 UTC  ` . Truncates higher levels of precision by rounding down to the beginning of the microsecond.

**Return Data Type**

`  INT64  `

**Examples**

``` text
SELECT UNIX_MICROS(TIMESTAMP "2008-12-25 15:30:00+00") AS micros;

/*------------------+
 | micros           |
 +------------------+
 | 1230219000000000 |
 +------------------*/
```

``` text
SELECT UNIX_MICROS(TIMESTAMP "1970-01-01 00:00:00.0000018+00") AS micros;

/*------------------+
 | micros           |
 +------------------+
 | 1                |
 +------------------*/
```

## `     UNIX_MILLIS    `

``` text
UNIX_MILLIS(timestamp_expression)
```

**Description**

Returns the number of milliseconds since `  1970-01-01 00:00:00 UTC  ` . Truncates higher levels of precision by rounding down to the beginning of the millisecond.

**Return Data Type**

`  INT64  `

**Examples**

``` text
SELECT UNIX_MILLIS(TIMESTAMP "2008-12-25 15:30:00+00") AS millis;

/*---------------+
 | millis        |
 +---------------+
 | 1230219000000 |
 +---------------*/
```

``` text
SELECT UNIX_MILLIS(TIMESTAMP "1970-01-01 00:00:00.0018+00") AS millis;

/*---------------+
 | millis        |
 +---------------+
 | 1             |
 +---------------*/
```

## `     UNIX_SECONDS    `

``` text
UNIX_SECONDS(timestamp_expression)
```

**Description**

Returns the number of seconds since `  1970-01-01 00:00:00 UTC  ` . Truncates higher levels of precision by rounding down to the beginning of the second.

**Return Data Type**

`  INT64  `

**Examples**

``` text
SELECT UNIX_SECONDS(TIMESTAMP "2008-12-25 15:30:00+00") AS seconds;

/*------------+
 | seconds    |
 +------------+
 | 1230219000 |
 +------------*/
```

``` text
SELECT UNIX_SECONDS(TIMESTAMP "1970-01-01 00:00:01.8+00") AS seconds;

/*------------+
 | seconds    |
 +------------+
 | 1          |
 +------------*/
```

## Supplemental materials

### How time zones work with timestamp functions

A timestamp represents an absolute point in time, independent of any time zone. However, when a timestamp value is displayed, it's usually converted to a human-readable format consisting of a civil date and time (YYYY-MM-DD HH:MM:SS) and a time zone. This isn't the internal representation of the `  TIMESTAMP  ` ; it's only a human-understandable way to describe the point in time that the timestamp represents.

Some timestamp functions have a time zone argument. A time zone is needed to convert between civil time (YYYY-MM-DD HH:MM:SS) and the absolute time represented by a timestamp. A function like `  PARSE_TIMESTAMP  ` takes an input string that represents a civil time and returns a timestamp that represents an absolute time. A time zone is needed for this conversion. A function like `  EXTRACT  ` takes an input timestamp (absolute time) and converts it to civil time in order to extract a part of that civil time. This conversion requires a time zone. If no time zone is specified, the default time zone, America/Los\_Angeles, is used.

Certain date and timestamp functions allow you to override the default time zone and specify a different one. You can specify a time zone by either supplying the time zone name (for example, `  America/Los_Angeles  ` ) or time zone offset from UTC (for example, -08).

To learn more about how time zones work with the `  TIMESTAMP  ` type, see [Time zones](/spanner/docs/reference/standard-sql/data-types#time_zones) .
