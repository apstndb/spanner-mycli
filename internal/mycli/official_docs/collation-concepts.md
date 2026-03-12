GoogleSQL for Spanner supports collation. Collation defines rules to sort and compare strings in an [`  ORDER BY  ` operation](/spanner/docs/reference/standard-sql/query-syntax#order_by_clause) .

By default, GoogleSQL sorts strings case-sensitively. This means that `  a  ` and `  A  ` are treated as different letters, and `  Z  ` would come before `  a  ` .

**Example default sorting:** Apple, Zebra, apple

By contrast, collation lets you sort and compare strings case-insensitively or according to specific language rules.

**Example case-insensitive collation:** Apple, apple, Zebra

To customize collation in the operation, include the [`  COLLATE  ` clause](/spanner/docs/reference/standard-sql/query-syntax#collate_clause) with a [collation specification](#collate_spec_details) .

Collation is useful when you need fine-tuned control over how values are sorted, joined, or grouped in tables.

## Where you can assign a collation specification

In the `  ORDER BY  ` clause, you can specify a collation specification for a collation-supported column. This overrides any collation specifications set previously.

For example:

``` text
SELECT Place
FROM Locations
ORDER BY Place COLLATE "und:ci"
```

### Query statements

You can assign a collation specification to the following query statements.

<table>
<thead>
<tr class="header">
<th>Type</th>
<th>Support</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Sorting</td>
<td><a href="/spanner/docs/reference/standard-sql/query-syntax#order_by_clause"><code dir="ltr" translate="no">        ORDER BY       </code> clause</a></td>
</tr>
</tbody>
</table>

## Collation specification details

A collation specification determines how strings are sorted and compared in [collation-supported operations](#collate_operations) . You can define a collation specification for [collation-supported types](#collate_define) . These types of collation specifications are available:

  - [Unicode collation specification](#unicode_collation)

If a collation specification isn't defined, the default collation specification is used. To learn more, see the next section.

### Default collation specification

When a collation specification isn't assigned or is empty, the ordering behavior is identical to `  'unicode'  ` collation, which you can learn about in the [Unicode collation specification](#unicode_collation) .

### Unicode collation specification

``` text
collation_specification:
  'language_tag[:collation_attribute]'
```

A unicode collation specification indicates that the operation should use the [Unicode Collation Algorithm](http://www.unicode.org/reports/tr10/) to sort and compare strings. The collation specification can be a `  STRING  ` literal or a query parameter.

#### The language tag

The language tag determines how strings are generally sorted and compared. Allowed values for `  language_tag  ` are:

  - A standard locale string: This name is usually two or three letters that represent the language, optionally followed by an underscore or dash and two letters that represent the region — for example, `  en_US  ` . These names are defined by the [Common Locale Data Repository (CLDR)](https://www.unicode.org/reports/tr35/#Unicode_locale_identifier) .
  - `  und  ` : A locale string representing the *undetermined* locale. `  und  ` is a special language tag defined in the [IANA language subtag registry](https://www.iana.org/assignments/language-subtag-registry/language-subtag-registry) and used to indicate an undetermined locale. This is also known as the *root* locale and can be considered the *default* Unicode collation. It defines a reasonable, locale agnostic collation. It differs significantly from `  unicode  ` .
  - `  unicode  ` : Returns data in Unicode code point order, which is identical to the ordering behavior when `  COLLATE  ` isn't used. The sort order will look largely arbitrary to human users.

#### The collation attribute

In addition to the language tag, the unicode collation specification can have an optional `  collation_attribute  ` , which enables additional rules for sorting and comparing strings. Allowed values are:

  - `  ci  ` : Collation is case-insensitive.
  - `  cs  ` : Collation is case-sensitive. By default, `  collation_attribute  ` is implicitly `  cs  ` .

If you're using the `  unicode  ` language tag with a collation attribute, these caveats apply:

  - `  unicode:cs  ` is identical to `  unicode  ` .
  - `  unicode:ci  ` is identical to `  und:ci  ` . It's recommended to migrate `  unicode:ci  ` to `  binary  ` .

#### Collation specification example

This is what the `  ci  ` collation attribute looks like when used with the `  und  ` language tag in the `  ORDER BY  ` clause:

``` text
SELECT Place
FROM Locations
ORDER BY Place COLLATE 'und:ci'
```

#### Caveats

  - Differing strings can be considered equal. For instance, `  ẞ  ` (LATIN CAPITAL LETTER SHARP S) is considered equal to `  'SS'  ` in some contexts. The following expressions both evaluate to `  TRUE  ` :
    
      - `  COLLATE('ẞ', 'und:ci') > COLLATE('SS', 'und:ci')  `
      - `  COLLATE('ẞ1', 'und:ci') < COLLATE('SS2', 'und:ci')  `
    
    This is similar to how case insensitivity works.

  - In search operations, strings with different lengths could be considered equal. To ensure consistency, collation should be used without search tailoring.

  - There are a wide range of unicode code points (punctuation, symbols, etc), that are treated as if they aren't there. So strings with and without them are sorted identically. For example, the format control code point `  U+2060  ` is ignored when the following strings are sorted:
    
    ``` text
    SELECT *
    FROM UNNEST([
      'oran\u2060ge1',
      '\u2060orange2',
      'orange3'
    ]) AS fruit
    ORDER BY fruit COLLATE 'und'
    
    /*---------+
    | fruit   |
    +---------+
    | orange1 |
    | orange2 |
    | orange3 |
    +---------*/
    ```

  - Ordering *may* change. The Unicode specification of the `  und  ` collation can change occasionally, which can affect sorting order. If you need a stable sort order that's guaranteed to never change, use `  unicode  ` collation.
