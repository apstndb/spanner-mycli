GoogleSQL for Spanner supports the following format elements.

## Format elements for date and time parts

Many GoogleSQL parsing and formatting functions rely on a format string to describe the format of parsed or formatted values. A format string represents the textual form of date and time and contains separate format elements that are applied left-to-right.

These functions use format strings:

  - [`  FORMAT_DATE  `](/spanner/docs/reference/standard-sql/date_functions#format_date)
  - [`  FORMAT_TIMESTAMP  `](/spanner/docs/reference/standard-sql/timestamp_functions#format_timestamp)
  - [`  PARSE_DATE  `](/spanner/docs/reference/standard-sql/date_functions#parse_date)
  - [`  PARSE_TIMESTAMP  `](/spanner/docs/reference/standard-sql/timestamp_functions#parse_timestamp)

Format strings generally support the following elements:

<table>
<colgroup>
<col style="width: 25%" />
<col style="width: 25%" />
<col style="width: 25%" />
<col style="width: 25%" />
</colgroup>
<thead>
<tr class="header">
<th>Format element</th>
<th>Type</th>
<th>Description</th>
<th>Example</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td><code dir="ltr" translate="no">       %A      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The full weekday name (English).</td>
<td><code dir="ltr" translate="no">       Wednesday      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %a      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The abbreviated weekday name (English).</td>
<td><code dir="ltr" translate="no">       Wed      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %B      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The full month name (English).</td>
<td><code dir="ltr" translate="no">       January      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %b      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The abbreviated month name (English).</td>
<td><code dir="ltr" translate="no">       Jan      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %C      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The century (a year divided by 100 and truncated to an integer) as a decimal number (00-99).</td>
<td><code dir="ltr" translate="no">       20      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %c      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The date and time representation (English).</td>
<td><code dir="ltr" translate="no">       Wed Jan 20 21:47:00 2021      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %D      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The date in the format %m/%d/%y.</td>
<td><code dir="ltr" translate="no">       01/20/21      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %d      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The day of the month as a decimal number (01-31).</td>
<td><code dir="ltr" translate="no">       20      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %e      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The day of month as a decimal number (1-31); single digits are preceded by a space.</td>
<td><code dir="ltr" translate="no">       20      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %F      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The date in the format %Y-%m-%d.</td>
<td><code dir="ltr" translate="no">       2021-01-20      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %G      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The <a href="https://en.wikipedia.org/wiki/ISO_8601">ISO 8601</a> year with century as a decimal number. Each ISO year begins on the Monday before the first Thursday of the Gregorian calendar year. Note that %G and %Y may produce different results near Gregorian year boundaries, where the Gregorian year and ISO year can diverge.</td>
<td><code dir="ltr" translate="no">       2021      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %g      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The <a href="https://en.wikipedia.org/wiki/ISO_8601">ISO 8601</a> year without century as a decimal number (00-99). Each ISO year begins on the Monday before the first Thursday of the Gregorian calendar year. Note that %g and %y may produce different results near Gregorian year boundaries, where the Gregorian year and ISO year can diverge.</td>
<td><code dir="ltr" translate="no">       21      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %H      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The hour (24-hour clock) as a decimal number (00-23).</td>
<td><code dir="ltr" translate="no">       21      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %h      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The abbreviated month name (English).</td>
<td><code dir="ltr" translate="no">       Jan      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %I      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The hour (12-hour clock) as a decimal number (01-12).</td>
<td><code dir="ltr" translate="no">       09      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %j      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The day of the year as a decimal number (001-366).</td>
<td><code dir="ltr" translate="no">       020      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %k      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The hour (24-hour clock) as a decimal number (0-23); single digits are preceded by a space.</td>
<td><code dir="ltr" translate="no">       21      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %l      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The hour (12-hour clock) as a decimal number (1-12); single digits are preceded by a space.</td>
<td><code dir="ltr" translate="no">       9      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %M      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The minute as a decimal number (00-59).</td>
<td><code dir="ltr" translate="no">       47      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %m      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The month as a decimal number (01-12).</td>
<td><code dir="ltr" translate="no">       01      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %n      </code></td>
<td>All</td>
<td>A newline character.</td>
<td></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %P      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>When formatting, this is either am or pm.<br />
This can't be used with parsing. Instead, use %p.<br />
</td>
<td><code dir="ltr" translate="no">       pm      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %p      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>When formatting, this is either AM or PM.<br />
When parsing, this can be used with am, pm, AM, or PM.<br />
</td>
<td><code dir="ltr" translate="no">       PM      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %Q      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The quarter as a decimal number (1-4).</td>
<td><code dir="ltr" translate="no">       1      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %R      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The time in the format %H:%M.</td>
<td><code dir="ltr" translate="no">       21:47      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %S      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The second as a decimal number (00-60).</td>
<td><code dir="ltr" translate="no">       00      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %s      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The number of seconds since 1970-01-01 00:00:00. Always overrides all other format elements, independent of where %s appears in the string. If multiple %s elements appear, then the last one takes precedence.</td>
<td><code dir="ltr" translate="no">       1611179220      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %T      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The time in the format %H:%M:%S.</td>
<td><code dir="ltr" translate="no">       21:47:00      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %t      </code></td>
<td>All</td>
<td>A tab character.</td>
<td></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %U      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The week number of the year (Sunday as the first day of the week) as a decimal number (00-53).</td>
<td><code dir="ltr" translate="no">       03      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %u      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The weekday (Monday as the first day of the week) as a decimal number (1-7).</td>
<td><code dir="ltr" translate="no">       3      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %V      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The <a href="https://en.wikipedia.org/wiki/ISO_week_date">ISO 8601</a> week number of the year (Monday as the first day of the week) as a decimal number (01-53). If the week containing January 1 has four or more days in the new year, then it's week 1; otherwise it's week 53 of the previous year, and the next week is week 1.</td>
<td><code dir="ltr" translate="no">       03      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %W      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The week number of the year (Monday as the first day of the week) as a decimal number (00-53).</td>
<td><code dir="ltr" translate="no">       03      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %w      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The weekday (Sunday as the first day of the week) as a decimal number (0-6).</td>
<td><code dir="ltr" translate="no">       3      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %X      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The time representation in HH:MM:SS format.</td>
<td><code dir="ltr" translate="no">       21:47:00      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %x      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The date representation in MM/DD/YY format.</td>
<td><code dir="ltr" translate="no">       01/20/21      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %Y      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The year with century as a decimal number.</td>
<td><code dir="ltr" translate="no">       2021      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %y      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The year without century as a decimal number (00-99), with an optional leading zero. Can be mixed with %C. If %C isn't specified, years 00-68 are 2000s, while years 69-99 are 1900s.</td>
<td><code dir="ltr" translate="no">       21      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %Z      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The time zone name.</td>
<td><code dir="ltr" translate="no">       UTC-5      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %z      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>The offset from the Prime Meridian in the format +HHMM or -HHMM as appropriate, with positive values representing locations east of Greenwich.</td>
<td><code dir="ltr" translate="no">       -0500      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %%      </code></td>
<td>All</td>
<td>A single % character.</td>
<td><code dir="ltr" translate="no">       %      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %Ez      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>RFC 3339-compatible numeric time zone (+HH:MM or -HH:MM).</td>
<td><code dir="ltr" translate="no">       -05:00      </code></td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %E&lt;number&gt;S      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>Seconds with &lt;number&gt; digits of fractional precision.</td>
<td><code dir="ltr" translate="no">       00.000 for %E3S      </code></td>
</tr>
<tr class="even">
<td><code dir="ltr" translate="no">       %E*S      </code></td>
<td><code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>Seconds with full fractional precision (a literal '*').</td>
<td>00.123456789</td>
</tr>
<tr class="odd">
<td><code dir="ltr" translate="no">       %E4Y      </code></td>
<td><code dir="ltr" translate="no">        DATE       </code><br />
<code dir="ltr" translate="no">        TIMESTAMP       </code><br />
</td>
<td>Four-character years (0001 ... 9999). Note that %Y produces as many characters as it takes to fully render the year.</td>
<td><code dir="ltr" translate="no">       2021      </code></td>
</tr>
</tbody>
</table>

Examples:

``` text
SELECT FORMAT_DATE("%b-%d-%Y", DATE "2008-12-25") AS formatted;

/*-------------+
 | formatted   |
 +-------------+
 | Dec-25-2008 |
 +-------------*/
```

``` text
SELECT FORMAT_TIMESTAMP("%b %Y %Ez", TIMESTAMP "2008-12-25 15:30:00+00")
  AS formatted;

/*-----------------+
 | formatted       |
 +-----------------+
 | Dec 2008 +00:00 |
 +-----------------*/
```

``` text
SELECT PARSE_DATE("%Y%m%d", "20081225") AS parsed;

/*------------+
 | parsed     |
 +------------+
 | 2008-12-25 |
 +------------*/
```

``` text
SELECT PARSE_TIMESTAMP("%c", "Thu Dec 25 07:30:00 2008") AS parsed;

-- Display of results may differ, depending upon the environment and
-- time zone where this query was executed.
/*------------------------+
 | parsed                 |
 +------------------------+
 | 2008-12-25T15:30:00Z   |
 +------------------------*/
```
