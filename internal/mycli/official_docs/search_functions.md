GoogleSQL for Spanner supports the following search functions.

## Categories

The search functions are grouped into the following categories, based on their behavior:

<table>
<colgroup>
<col style="width: 33%" />
<col style="width: 33%" />
<col style="width: 33%" />
</colgroup>
<thead>
<tr class="header">
<th>Category</th>
<th>Functions</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>Indexing</td>
<td><a href="#token"><code dir="ltr" translate="no">        TOKEN       </code></a><br />
<a href="#tokenize_bool"><code dir="ltr" translate="no">        TOKENIZE_BOOL       </code></a><br />
<a href="#tokenize_fulltext"><code dir="ltr" translate="no">        TOKENIZE_FULLTEXT       </code></a><br />
<a href="#tokenize_json"><code dir="ltr" translate="no">        TOKENIZE_JSON       </code></a><br />
<a href="#tokenize_ngrams"><code dir="ltr" translate="no">        TOKENIZE_NGRAMS       </code></a><br />
<a href="#tokenize_number"><code dir="ltr" translate="no">        TOKENIZE_NUMBER       </code></a><br />
<a href="#tokenize_substring"><code dir="ltr" translate="no">        TOKENIZE_SUBSTRING       </code></a><br />
<a href="#tokenlist_concat"><code dir="ltr" translate="no">        TOKENLIST_CONCAT       </code></a><br />
</td>
<td>Functions that you can use to create search indexes.</td>
</tr>
<tr class="even">
<td>Retrieval and presentation</td>
<td><a href="#score"><code dir="ltr" translate="no">        SCORE       </code></a><br />
<a href="#score_ngrams"><code dir="ltr" translate="no">        SCORE_NGRAMS       </code></a><br />
<a href="#search_fulltext"><code dir="ltr" translate="no">        SEARCH       </code></a><br />
<a href="#search_ngrams"><code dir="ltr" translate="no">        SEARCH_NGRAMS       </code></a><br />
<a href="#search_substring"><code dir="ltr" translate="no">        SEARCH_SUBSTRING       </code></a><br />
<a href="#snippet"><code dir="ltr" translate="no">        SNIPPET       </code></a><br />
</td>
<td>Functions that you can use to search for data, score the search result, or format the search result.</td>
</tr>
<tr class="odd">
<td>Debugging</td>
<td><a href="#debug_tokenlist"><code dir="ltr" translate="no">        DEBUG_TOKENLIST       </code></a><br />
</td>
<td>Functions that you can use for debugging.</td>
</tr>
</tbody>
</table>

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
<td><a href="/spanner/docs/reference/standard-sql/search_functions#debug_tokenlist"><code dir="ltr" translate="no">        DEBUG_TOKENLIST       </code></a></td>
<td>Displays a human-readable representation of tokens present in the <code dir="ltr" translate="no">       TOKENLIST      </code> value for debugging purposes.</td>
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
<td><a href="/spanner/docs/reference/standard-sql/search_functions#snippet"><code dir="ltr" translate="no">        SNIPPET       </code></a></td>
<td>Gets a list of snippets that match a full-text search query.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#token"><code dir="ltr" translate="no">        TOKEN       </code></a></td>
<td>Constructs an exact match <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing a <code dir="ltr" translate="no">       BYTE      </code> or <code dir="ltr" translate="no">       STRING      </code> value verbatim to accelerate exact match expressions in SQL.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_bool"><code dir="ltr" translate="no">        TOKENIZE_BOOL       </code></a></td>
<td>Constructs a boolean <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing a <code dir="ltr" translate="no">       BOOL      </code> value to accelerate boolean match expressions in SQL.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_fulltext"><code dir="ltr" translate="no">        TOKENIZE_FULLTEXT       </code></a></td>
<td>Constructs a full-text <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing text for full-text matching.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_json"><code dir="ltr" translate="no">        TOKENIZE_JSON       </code></a></td>
<td>Constructs a JSON <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing a <code dir="ltr" translate="no">       JSON      </code> value to accelerate JSON predicate expressions in SQL.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_ngrams"><code dir="ltr" translate="no">        TOKENIZE_NGRAMS       </code></a></td>
<td>Constructs an n-gram <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing a <code dir="ltr" translate="no">       STRING      </code> value for matching n-grams.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_number"><code dir="ltr" translate="no">        TOKENIZE_NUMBER       </code></a></td>
<td>Constructs a numeric <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing numeric values to accelerate numeric comparison expressions in SQL.</td>
</tr>
<tr class="even">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenize_substring"><code dir="ltr" translate="no">        TOKENIZE_SUBSTRING       </code></a></td>
<td>Constructs a substring <code dir="ltr" translate="no">       TOKENLIST      </code> value by tokenizing text for substring matching.</td>
</tr>
<tr class="odd">
<td><a href="/spanner/docs/reference/standard-sql/search_functions#tokenlist_concat"><code dir="ltr" translate="no">        TOKENLIST_CONCAT       </code></a></td>
<td>Constructs a <code dir="ltr" translate="no">       TOKENLIST      </code> value by concatenating one or more <code dir="ltr" translate="no">       TOKENLIST      </code> values.</td>
</tr>
</tbody>
</table>

## `     DEBUG_TOKENLIST    `

``` text
DEBUG_TOKENLIST(tokenlist)
```

**Description**

Displays a human-readable representation of tokens present in a `  TOKENLIST  ` value for debugging purposes.

**Definitions**

  - `  tokenlist  ` : The `  TOKENLIST  ` value to display.

**Details**

**Note:** The returned `  STRING  ` value is intended solely for debugging purposes and its format is subject to change without notice.

The output of this function is dependent on the source of the `  TOKENLIST  ` value provided as input.

**Return type**

`  STRING  `

**Examples**

The following query illustrates how attributes and positions are represented:

  - In `  hello(boundary)  ` , `  hello  ` is the text of the token and `  boundary  ` is an attribute of the token.
  - Token `  db  ` has no attributes.
  - In `  [#world, world](boundary)  ` , `  #world  ` and `  world  ` are both tokens added to the tokenlist, at the same position. `  boundary  ` is the attribute for both of them. This can match either `  #world  ` or `  world  ` query terms.

<!-- end list -->

``` text
SELECT DEBUG_TOKENLIST(TOKENIZE_FULLTEXT('Hello DB #World')) AS Result;

/*------------------------------------------------+
 | Result                                         |
 +------------------------------------------------+
 | hello(boundary), db, [#world, world](boundary) |
 +------------------------------------------------*/
```

The following query illustrates how equality and range are represented:

  - `  ==1  ` and `  ==10  ` represent equality tokens for `  1  ` and `  10  ` .
  - `  [1, 1]  ` represents a range token with `  1  ` as the lower bound and `  1  ` as the upper bound.

<!-- end list -->

``` text
SELECT DEBUG_TOKENLIST(TOKENIZE_NUMBER([1, 10], min=> 1, max=>10)) AS Result;

/*--------------------------------------------------------------------------------+
 | Result                                                                         |
 +--------------------------------------------------------------------------------+
 | ==1, ==10, [1, 1], [1, 2], [1, 4], [1, 8], [9, 10], [9, 12], [9, 16], [10, 10] |
 +--------------------------------------------------------------------------------*/
```

## `     SCORE    `

``` text
SCORE(
  tokens,
  search_query
  [, dialect => { "rquery" | "words" | "words_phrase" } ]
  [, language_tag => value ]
  [, enhance_query => { TRUE | FALSE } ]
  [, options => value ]
)
```

**Description**

Calculates a relevance score of a `  TOKENLIST  ` for a full-text search query. The higher the score, the stronger the match.

**Definitions**

  - `  tokens  ` : A `  TOKENLIST  ` value that represents a list of full-text tokens.

  - `  search_query  ` : A `  STRING  ` value that represents a search query, which is interpreted based on the `  dialect  ` argument. For more information, see the [search query overview](/spanner/docs/full-text-search/query-overview#search-query) .

  - `  dialect  ` : A named argument with a `  STRING  ` value. The value determines how `  search_query  ` is understood and processed. If the value is `  NULL  ` or this argument isn't specified, `  rquery  ` is used by default. This function supports the following dialect values:
    
      - `  rquery  ` : The raw search query (or "rquery") using a domain-specific language (DSL). For more information, see [rquery syntax overview](/spanner/docs/full-text-search/query-overview#rquery) . For rquery syntax rules, see [rquery syntax](/spanner/docs/reference/standard-sql/search_functions#rquery-syntax) .
    
      - `  words  ` : Perform a conjunctive search, requiring all terms in `  search_query  ` to be present. For an overview, see [words dialect overview](/spanner/docs/full-text-search/query-overview#words) . For syntax rules, see [words syntax](/spanner/docs/reference/standard-sql/search_functions#words-syntax) .
    
      - `  words_phrase  ` : Perform a phrase search that requires all terms in `  search_query  ` to be adjacent and in order. For an overview, see [words phrase overview](/spanner/docs/full-text-search/query-overview#words-phrase) . For syntax rules, see [words\_phrase syntax](/spanner/docs/reference/standard-sql/search_functions#words-phrase-syntax) .

  - `  language_tag  ` : A named argument with a `  STRING  ` value. The value contains an [IETF BCP 47 language tag](https://en.wikipedia.org/wiki/IETF_language_tag) . You can use this tag to specify the language for `  search_query  ` . If the value for this argument is `  NULL  ` , this function doesn't use a specific language. If this argument isn't specified, `  NULL  ` is used by default.

  - `  enhance_query  ` : A named argument with a `  BOOL  ` value. The value determines whether to enhance the search query. For example, if `  enhance_query  ` is enabled, a search query containing the term `  classic  ` can expand to include similar terms such as `  classical  ` . If the `  enhance_query  ` call times out, the search query proceeds without enhancement. However, if the query includes `  @{require_enhance_query=true} SELECT ...  ` , a timeout causes the entire query to fail instead. The default timeout for query enhancement is 500 ms, which you can override using a hint like `  @{enhance_query_timeout_ms=200} SELECT ...  ` .  
    Google is continuously improving the query enhancement algorithms. As a result, a query with `  enhance_query=>true  ` might yield slightly different results over time.
    
      - If `  TRUE  ` , the search query is enhanced to improve search quality.
    
      - If `  FALSE  ` (default), the search query isn't enhanced.

  - `  options  ` : A named argument with a `  JSON  ` value. The value represents the fine-tuning for the search scoring.
    
      - `  bigram_weight  ` : A multiplier for bigrams, which have matching terms adjacent to each other. The default is 2.0.
    
      - `  idf_weight  ` : A multiplier for term commonality. Hits on rare terms score relatively higher than hits on common terms. The default is 1.0.
    
      - `  token_category_weights  ` : A multiplier for each HTML category. The available categories are: `  small  ` , `  medium  ` , `  large  ` , `  title  ` .
    
      - `  version  ` : A distinct release of the Scorer that bundles a specific set of active features and default parameter values. The available versions are: `  1  ` , `  2  ` , and the default is `  1  ` . For example: `  options=> JSON '{"version": 2}'  `

**Details**

  - This function must reference a full-text `  TOKENLIST  ` column in a table that is also indexed in a search index. To add a full-text `  TOKENLIST  ` column to a table and to a search index, see the examples for this function.
  - This function requires the `  SEARCH  ` function in the same SQL query.
  - This function returns `  0  ` when `  tokens  ` or `  search_query  ` is `  NULL  ` .

**Versions**

The `  SCORE  ` algorithm is periodically updated. After a short evaluation period, the default behavior updates to the newest version. You are encouraged to leave the version unspecified so that your database can benefit from improvements to the `  SCORE  ` algorithm. However, you can set the version number in the `  options  ` argument to retain old behavior.

  - **2** (2025-08):
    
      - When `  enhance_query  ` is true, hits on synonyms are now demoted based on confidence in the synonym's accuracy.
    
      - Improved the algorithm that limits each query term's maximum contribution to the overall score.
    
      - Fixed an issue where documents with exactly one hit for a query term received a lower score than intended.
    
      - Fixed an issue where query terms under an "OR" were not weighted correctly, especially when `  enhance_query  ` was used.

  - **1** ( *Default* ): The initial version.

**Return type**

`  FLOAT64  `

**Examples**

The following examples reference a table called `  Albums  ` and a search index called `  AlbumsIndex  ` .

The `  Albums  ` table contains a column called `  DescriptionTokens  ` , which tokenizes the input added to the `  Description  ` column, and then saves those tokens in the `  DescriptionTokens  ` column. Finally, `  AlbumsIndex  ` indexes `  DescriptionTokens  ` . Once `  DescriptionTokens  ` is indexed, it can be used with the `  SCORE  ` function.

``` text
CREATE TABLE Albums (
  SingerId INT64 NOT NULL,
  AlbumId INT64 NOT NULL,
  Description STRING(MAX),
  DescriptionTokens TOKENLIST AS (TOKENIZE_FULLTEXT(Description)) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(DescriptionTokens);

INSERT INTO Albums (SingerId, AlbumId, Description) VALUES (1, 1, 'classical album');
INSERT INTO Albums (SingerId, AlbumId, Description) VALUES (1, 2, 'classical and rock album');
```

The following query searches the column called `  Description  ` for a token called `  classical album  ` . If this token is found for singer ID `  1  ` , the matching `  Description  ` are returned with the corresponding score. Both `  classical album  ` and `  classical and rock album  ` have the terms `  classical  ` and `  album  ` , but the first one has a higher score because the terms are adjacent.

``` text
SELECT
  a.Description, SCORE(a.DescriptionTokens, 'classical album') AS Score
FROM
  Albums a
WHERE
  SEARCH(a.DescriptionTokens, 'classical album');

/*--------------------------+---------------------+
 | Description              | Score               |
 +--------------------------+---------------------+
 | classical album          | 1.2818930149078369  |
 | classical and rock album | 0.50003194808959961 |
 +--------------------------+---------------------*/
```

The following query is like the previous one. However, scores are boosted more with `  bigram_weight  ` on adjacent positions.

``` text
SELECT
  a.Description,
  SCORE(
    a.DescriptionTokens,
    'classical album',
    options=>JSON '{"bigram_weight": 3.0}'
  ) AS Score
FROM Albums a
WHERE SEARCH(a.DescriptionTokens, 'classical album');

/*--------------------------+---------------------+
 | Description              | Score               |
 +--------------------------+---------------------+
 | classical album          | 1.7417128086090088  |
 | classical and rock album | 0.50003194808959961 |
 +--------------------------+---------------------*/
```

The following query uses `  SCORE  ` in the `  ORDER BY  ` clause to get the row with the highest score.

``` text
SELECT a.Description
FROM Albums a
WHERE SEARCH(a.DescriptionTokens, 'classical album')
ORDER BY SCORE(a.DescriptionTokens, 'classical album') DESC
LIMIT 1;

/*--------------------------+
 | Description              |
 +--------------------------+
 | classical album          |
 +--------------------------*/
```

## `     SCORE_NGRAMS    `

``` text
SCORE_NGRAMS(
  tokens,
  ngrams_query
  [, language_tag => value ]
  [, algorithm => value ]
  [, array_aggregator => value ]
)
```

**Description**

Calculates a relevance score of a `  TOKENLIST  ` for a fuzzy search. The higher the score, the stronger the match.

**Definitions**

  - `  tokens  ` : A `  TOKENLIST  ` value that contains a list of n-gram tokens. This value must be a `  TOKENLIST  ` generated by either `  TOKENIZE_SUBSTRING  ` or `  TOKENIZE_NGRAMS  ` , and the tokenization function's `  value_to_tokenize  ` argument must be a column reference. A `  TOKENLIST  ` with an expression as `  value_to_tokenize  ` or a `  TOKENLIST  ` generated by `  TOKENLIST_CONCAT  ` isn't supported, such as `  TOKENIZE_SUBSTRING(REGEXP_REPLACE(col, 'foo', 'bar'))  ` or `  TOKENLIST_CONCAT([token1, token2])  ` . If using an expression as `  value_to_tokenize  ` or if a `  TOKENLIST  ` generated by `  TOKENLIST_CONCAT  ` is necessary, consider creating a generated column and then creating a `  TOKENLIST  ` from that generated column.

  - `  ngrams_query  ` : A `  STRING  ` value that represents a fuzzy search query.

  - `  language_tag  ` : A named argument with a `  STRING  ` value. The value contains an [IETF BCP 47 language tag](https://en.wikipedia.org/wiki/IETF_language_tag) . You can use this tag to specify the language for `  ngrams_query  ` . If the value for this argument is `  NULL  ` , this function doesn't use a specific language. If this argument isn't specified, `  NULL  ` is used by default.

  - `  algorithm  ` : A named argument with a `  STRING  ` value. The value specifies the scoring algorithm for the fuzzy search. The default value for this argument is `  trigrams  ` , and currently it's the only supported algorithm.
    
      - `  trigrams  ` : Generates trigrams (n-grams with size 3) without duplication from the query, then also generates trigrams without duplication from the source column of the `  tokens  ` . Matches are an intersection between query trigrams and source trigrams. The score is roughly calculated as `  (match_count / (query_trigrams + source_trigrams - match_count))  ` .

  - `  array_aggregator  ` : A named argument that determines how scoring is performed on array. This argument can be used only when tokenlist is from an array column. This argument uses a `  STRING  ` value. The default value for this argument is `  flatten  ` .
    
      - `  flatten  ` : Flattens the array column as a single string first, then calculates a score from the flattened string. More non-matching elements in the array makes the score lower.
    
      - `  max_element  ` : Scores each element separately, then returns the highest score.

**Details**

  - This function returns `  0  ` when `  tokens  ` or `  ngrams_query  ` is `  NULL  ` .
  - Unlike `  SEARCH_NGRAMS  ` , this function requires access to the source column of `  tokens  ` . Therefore, it's often advantageous to include the source column in `  SEARCH INDEX  ` 's `  STORING  ` clause, to avoid a join with the base table. Please see [index-only scans](/spanner/docs/secondary-indexes#storing-clause) .

**Return type**

`  FLOAT64  `

**Examples**

The following examples reference a table called `  Albums  ` and a search index called `  AlbumsIndex  ` .

The `  Albums  ` table contains a column `  DescriptionSubstrTokens  ` which tokenizes `  Description  ` column using `  TOKENIZE_SUBSTRING  ` . Finally, `  AlbumsIndex  ` stores `  Description  ` , so that the query below doesn't have to join with the base table.

``` text
CREATE TABLE Albums (
  AlbumId INT64 NOT NULL,
  Description STRING(MAX),
  DescriptionSubstrTokens TOKENLIST AS
    (TOKENIZE_SUBSTRING(Description, ngram_size_max=>3)) HIDDEN
) PRIMARY KEY (AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(DescriptionSubstrTokens)
  STORING(Description);

INSERT INTO Albums (AlbumId, Description) VALUES (1, 'rock album');
INSERT INTO Albums (AlbumId, Description) VALUES (2, 'classical album');
```

The following query scores `  Description  ` with `  clasic albun  ` , which is misspelled.

``` text
SELECT
  a.Description, SCORE_NGRAMS(a.DescriptionSubstrTokens, 'clasic albun') AS Score
FROM
  Albums a

/*-----------------+---------------------+
 | Description     | Score               |
 +-----------------+---------------------+
 | rock album      | 0.14285714285714285 |
 | classical album | 0.38095238095238093 |
 +-----------------+---------------------*/
```

The following query uses `  SCORE_NGRAMS  ` in the `  ORDER BY  ` clause to produce the row with the highest score.

``` text
SELECT a.Description
FROM Albums a
WHERE SEARCH_NGRAMS(a.DescriptionSubstrTokens, 'clasic albun')
ORDER BY SCORE_NGRAMS(a.DescriptionSubstrTokens, 'clasic albun') DESC
LIMIT 1

/*-----------------+
 | Description     |
 +-----------------+
 | classical album |
 +-----------------*/
```

## `     SEARCH    `

``` text
SEARCH(
  tokens,
  search_query
  [, dialect => { "rquery" | "words" | "words_phrase" } ]
  [, language_tag => value]
  [, enhance_query => { TRUE | FALSE }]
)
```

**Description**

Returns `  TRUE  ` if a full-text search query matches tokens.

**Definitions**

  - `  tokens  ` : A `  TOKENLIST  ` value that contains a list of full-text tokens. It must be a `  TOKENLIST  ` generated by either `  TOKENIZE_FULLTEXT  ` , or by concatenating `  TOKENLIST  ` s from `  TOKENIZE_FULLTEXT  ` using `  TOKENLIST_CONCAT  ` .

  - `  search_query  ` : A `  STRING  ` value that represents a search query, which is interpreted based on the `  dialect  ` argument. For more information, see the [search query overview](/spanner/docs/full-text-search/query-overview#search-query) .

  - `  dialect  ` : A named argument with a `  STRING  ` value. The value determines how `  search_query  ` is understood and processed. If the value is `  NULL  ` or this argument isn't specified, `  rquery  ` is used by default. This function supports the following dialect values:
    
      - `  rquery  ` : The raw search query (or "rquery") using a domain-specific language (DSL). For more information, see [rquery syntax overview](/spanner/docs/full-text-search/query-overview#rquery) . For rquery syntax rules, see [rquery syntax](/spanner/docs/reference/standard-sql/search_functions#rquery-syntax) .
    
      - `  words  ` : Perform a conjunctive search, requiring all terms in `  search_query  ` to be present. For an overview, see [words dialect overview](/spanner/docs/full-text-search/query-overview#words) . For syntax rules, see [words syntax](/spanner/docs/reference/standard-sql/search_functions#words-syntax) .
    
      - `  words_phrase  ` : Perform a phrase search that requires all terms in `  search_query  ` to be adjacent and in order. For an overview, see [words phrase overview](/spanner/docs/full-text-search/query-overview#words-phrase) . For syntax rules, see [words\_phrase syntax](/spanner/docs/reference/standard-sql/search_functions#words-phrase-syntax) .

  - `  language_tag  ` : A named argument with a `  STRING  ` value. The value contains an [IETF BCP 47 language tag](https://en.wikipedia.org/wiki/IETF_language_tag) . You can use this tag to specify the language for `  search_query  ` . If the value for this argument is `  NULL  ` , this function doesn't use a specific language. If this argument isn't specified, `  NULL  ` is used by default.

  - `  enhance_query  ` : A named argument with a `  BOOL  ` value. The value determines whether to enhance the search query. For example, if `  enhance_query  ` is enabled, a search query containing the term `  classic  ` can expand to include similar terms such as `  classical  ` . If the `  enhance_query  ` call times out, the search query proceeds without enhancement. However, if the query includes `  @{require_enhance_query=true} SELECT ...  ` , a timeout causes the entire query to fail instead. The default timeout for query enhancement is 500 ms, which you can override using a hint like `  @{enhance_query_timeout_ms=200} SELECT ...  ` .  
    Google is continuously improving the query enhancement algorithms. As a result, a query with `  enhance_query=>true  ` might yield slightly different results over time.
    
      - If `  TRUE  ` , the search query is enhanced to improve search quality.
    
      - If `  FALSE  ` (default), the search query isn't enhanced.

**Details**

  - Returns `  TRUE  ` if `  tokens  ` is a match for `  search_query  ` .
  - This function must reference a full-text `  TOKENLIST  ` column in a table that is also indexed in a search index. To add a full-text `  TOKENLIST  ` column to a table and to a search index, see the examples for this function.
  - This function returns `  NULL  ` when `  tokens  ` or `  search_query  ` is `  NULL  ` .
  - This function can only be used in the `  WHERE  ` clause of a SQL query.

**Search query syntax dialects**

Search query uses rquery syntax by default. You can specify other supported syntax dialects using the `  dialect  ` argument.

  - **rquery syntax (default)**
    
    The rquery dialect follows these rules:
    
      - Multiple terms imply `  AND  ` . For example, "big time" is equivalent to `  big AND time  ` .
    
      - The `  OR  ` operator implies disjunction between two terms, such as `  big OR time  ` . The predicate `  SEARCH(tl, 'big time OR fast car')  ` is equivalent to:
        
        ``` text
        SEARCH(tl, 'big')
        AND (SEARCH(tl, 'time')
            OR SEARCH(tl, 'fast'))
        AND SEARCH(tl, 'car');
        ```
        
        `  OR  ` only applies to the two adjacent terms so the search expression `  big time OR fast car  ` searches for all the documents that have the terms `  big  ` and `  car  ` and either `  time  ` or `  fast  ` .
        
        `  The OR  ` operator is case sensitive.
        
        The pipe character ( `  |  ` ) is a shortcut for `  OR  ` .
    
      - Double quotes mean a phrase search. For example, the rquery `  "fast car"  ` matches "You got a fast car", but doesn't match "driving fast in my car".
    
      - The `  AROUND  ` operator matches terms that are within a certain distance of each other, and in the same order (the default is five tokens). For example, the rquery `  fast AROUND car  ` matches "driving fast in my car", but doesn't match "driving fast in his small shiny metal Italian car". The default is to match terms separated by, at most, five positions. To adjust the distance, pass an argument to the `  AROUND  ` operator. supports two syntaxes for `  AROUND  ` :
        
          - `  fast AROUND(10) car  `
          - `  fast AROUND 10 car  `
    
      - The `  AROUND  ` operator is case sensitive.
    
      - Negation of a single term is expressed with a dash ( `  -  ` ). For example `  -dog  ` matches all documents that don't contain the term `  dog  ` .
    
      - Punctuation is generally ignored. For example, "Fast Car\!" is equivalent to "Fast Car".
    
      - Search is case insensitive. For example, "Fast Car" matches "fast car".
    
    The following table explains the meaning of various rquery strings:
    
    <table>
    <thead>
    <tr class="header">
    <th>rquery</th>
    <th>Explanation</th>
    </tr>
    </thead>
    <tbody>
    <tr class="odd">
    <td><code dir="ltr" translate="no">         Miles Davis        </code></td>
    <td>Matches documents that contain both terms "Miles" and "Davis".</td>
    </tr>
    <tr class="even">
    <td><code dir="ltr" translate="no">         Miles OR Davis        </code></td>
    <td>Matches documents that contain at least one of the terms "Miles" and "Davis".</td>
    </tr>
    <tr class="odd">
    <td><code dir="ltr" translate="no">         -Davis        </code></td>
    <td>Matches all documents that don't contain the term "Davis".</td>
    </tr>
    <tr class="even">
    <td><code dir="ltr" translate="no">         "Miles Davis" -"Miles Jaye"        </code></td>
    <td>Matches documents that contain two adjacent terms "Miles" and "Davis", but don't contain adjacent "Miles" and "Jaye". For example, this query matches "I saw Miles Davis last night and Jaye earlier today", but doesn't match "I saw Miles Davis and Miles Jaye perform together".</td>
    </tr>
    <tr class="odd">
    <td><code dir="ltr" translate="no">         Davis|Jaye        </code></td>
    <td>This is the same as <code dir="ltr" translate="no">         Davis OR Jaye        </code> .</td>
    </tr>
    <tr class="even">
    <td><code dir="ltr" translate="no">         and OR or        </code></td>
    <td>Matches documents that have either the term "and" or the term "or" (the <code dir="ltr" translate="no">         OR        </code> operator must be uppercase)</td>
    </tr>
    </tbody>
    </table>

  - **words syntax**
    
    The words dialect follows these rules:
    
      - Multiple terms imply `  AND  ` . For example, "red yellow blue" is equivalent to `  red AND yellow AND blue  ` .
      - Punctuation is generally ignored. For example, "red\*yellow%blue" is equivalent to "red yellow blue".
      - Search is case insensitive.

  - **words\_phrase syntax**
    
    The words\_phrase dialect follows these rules:
    
      - Multiple terms imply a phrase. For example, the query "colorful rainbow" matches "There is a colorful rainbow", but doesn't match "The rainbow is colorful".
      - Punctuation is generally ignored. For example, "colorful rainbow\!" is equivalent to "colorful rainbow".
      - Search is case insensitive.

**Return type**

`  BOOL  `

**Examples**

The following examples reference a table called `  Albums  ` and a search index called `  AlbumsIndex  ` .

The `  Albums  ` table contains a column called `  DescriptionTokens  ` , which tokenizes the `  Description  ` column using `  TOKENIZE_FULLTEXT  ` , and then saves those tokens in the `  DescriptionTokens  ` column. Finally, `  AlbumsIndex  ` indexes `  DescriptionTokens  ` . Once `  DescriptionTokens  ` is indexed, it can be used with the `  SEARCH  ` function.

``` text
CREATE TABLE Albums (
  SingerId INT64 NOT NULL,
  AlbumId INT64 NOT NULL,
  Description STRING(MAX),
  DescriptionTokens TOKENLIST AS (TOKENIZE_FULLTEXT(Description)) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(DescriptionTokens)
PARTITION BY SingerId;

INSERT INTO Albums (SingerId, AlbumId, Description) VALUES (1, 1, 'rock album');
INSERT INTO Albums (SingerId, AlbumId, Description) VALUES (1, 2, 'classical album');
```

The following query searches the column called `  Description  ` for a token called `  classical  ` . If this token is found for singer ID `  1  ` , the matching rows are returned.

``` text
SELECT a.AlbumId, a.Description
FROM Albums a
WHERE a.SingerId = 1 AND SEARCH(a.DescriptionTokens, 'classical');

/*---------------------------+
 | AlbumId | Description     |
 +---------------------------+
 | 2       | classical album |
 +---------------------------*/
```

The following query is like the previous one. However, if `  Description  ` contains the `  classical  ` or `  rock  ` token, the matching rows are returned.

``` text
SELECT a.AlbumId, a.Description
FROM Albums a
WHERE a.SingerId = 1 AND SEARCH(a.DescriptionTokens, 'classical OR rock');

/*---------------------------+
 | AlbumId | Description     |
 +---------------------------+
 | 2       | classical album |
 | 1       | rock album      |
 +---------------------------*/
```

The following query is like the previous ones. However, if `  Description  ` contains the `  classic  ` and `  albums  ` token, the matching rows are returned. When `  enhance_query  ` is enabled, it includes similar matches of `  classical  ` and `  album  ` .

``` text
SELECT a.AlbumId, a.Description
FROM Albums a
WHERE a.SingerId = 1 AND SEARCH(a.DescriptionTokens, 'classic albums', enhance_query => TRUE);

/*---------------------------+
 | AlbumId | Description     |
 +---------------------------+
 | 2       | classical album |
 +---------------------------*/
```

## `     SEARCH_NGRAMS    `

``` text
SEARCH_NGRAMS(
  tokens,
  ngrams_query
  [, language_tag => value ]
  [, min_ngrams => value ]
  [, min_ngrams_percent => value ]
)
```

**Description**

Checks whether enough n-grams match the tokens in a fuzzy search.

**Definitions**

  - `  tokens  ` : A `  TOKENLIST  ` value that contains a list of n-gram tokens. It must be a `  TOKENLIST  ` generated by `  TOKENIZE_SUBSTRING  ` , `  TOKENIZE_NGRAMS  ` , or by concatenating `  TOKENLIST  ` s from `  TOKENIZE_SUBSTRING  ` using `  TOKENLIST_CONCAT  ` .

  - `  ngrams_query  ` : A `  STRING  ` value that represents a fuzzy search query. This function generates n-gram query terms from this value, using the same tokenization method as what was used to produce `  tokens  ` (for example, if `  TOKENIZE_SUBSTRING  ` was used, `  ngrams_query  ` is split into lower-cased words before producing n-grams), with `  token  ` 's `  ngram_size_max  ` as n-gram size.

  - `  language_tag  ` : A named argument with a `  STRING  ` value. The value contains an [IETF BCP 47 language tag](https://en.wikipedia.org/wiki/IETF_language_tag) . You can use this tag to specify the language for `  ngrams_query  ` . If the value for this argument is `  NULL  ` , this function doesn't use a specific language. If this argument isn't specified, `  NULL  ` is used by default.

  - `  min_ngrams  ` : A named argument with an `  INT64  ` value. The value specifies the minimum number of n-grams in `  ngrams_query  ` that have to match in order for `  SEARCH_NGRAMS  ` to return `  true  ` . This only counts distinct n-grams and ignores repeating n-grams. The default value for this argument is `  2  ` .

  - `  min_ngrams_percent  ` : A named argument with a `  FLOAT64  ` value. The value specifies the minimum percentage of n-grams in `  ngrams_query  ` that have to match in order for `  SEARCH_NGRAMS  ` to return `  true  ` . This only counts distinct n-grams and ignores repeating n-grams.

**Details**

  - This function must reference a substring or n-grams `  TOKENLIST  ` column in a table that's also indexed in a search index.
  - This function returns `  NULL  ` when `  tokens  ` or `  ngrams_query  ` is `  NULL  ` .
  - This function returns `  false  ` if the length of `  ngrams_query  ` is smaller than `  ngram_size_min  ` of `  tokens  ` .
  - This function can only be used in the `  WHERE  ` clause of a SQL query.

**Return type**

`  BOOL  `

**Examples**

The following examples reference a table called `  Albums  ` and a search index called `  AlbumsIndex  ` .

The `  Albums  ` table contains columns `  DescriptionSubstrTokens  ` and `  DescriptionNgramsTokens  ` which tokenize a `  Description  ` column using `  TOKENIZE_SUBSTRING  ` and `  TOKENIZE_NGRAMS  ` , respectively. Finally, `  AlbumsIndex  ` indexes `  DescriptionSubstrTokens  ` and `  DescriptionNgramsTokens  ` .

``` text
CREATE TABLE Albums (
  AlbumId INT64 NOT NULL,
  Description STRING(MAX),
  DescriptionSubstrTokens TOKENLIST AS
    (TOKENIZE_SUBSTRING(Description, ngram_size_min=>3, ngram_size_max=>3)) HIDDEN,
  DescriptionNgramsTokens TOKENLIST AS
    (TOKENIZE_NGRAMS(Description, ngram_size_min=>3, ngram_size_max=>3)) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(DescriptionSubstrTokens, DescriptionNgramsTokens);

INSERT INTO Albums (AlbumId, Description) VALUES (1, 'rock album');
INSERT INTO Albums (AlbumId, Description) VALUES (2, 'classical album');
INSERT INTO Albums (AlbumId, Description) VALUES (3, 'last note');
```

The following query searches the column `  Description  ` for `  clasic  ` . The query is misspelled, so querying with `  SEARCH_SUBSTRING(a.DescriptionSubstrTokens, 'clasic')  ` doesn't return a row, but the n-grams search is able to find similar matches.

`  SEARCH_NGRAMS  ` first transforms the query `  clasic  ` into n-grams of size 3 (the value of `  DescriptionSubstrTokens  ` 's `  ngram_size_max  ` ), producing `  ['asi', 'cla', 'las', 'sic']  ` . Then it finds rows that have at least two of these n-grams (the default value for `  min_ngrams  ` ) in the `  DescriptionSubstrTokens  ` column.

``` text
SELECT
  a.AlbumId, a.Description
FROM
  Albums a
WHERE
  SEARCH_NGRAMS(a.DescriptionSubstrTokens, 'clasic');

/*---------------------------+
 | AlbumId | Description     |
 +---------------------------+
 | 2       | classical album |
 +---------------------------*/
```

If we change the `  min_ngrams  ` to 1, then the query will also return the row with `  last  ` which has one n-gram match with `  las  ` . This example illustrates the decreased relevancy of the returned results when this parameter is set low.

``` text
SELECT
  a.AlbumId, a.Description
FROM
  Albums a
WHERE
  SEARCH_NGRAMS(a.DescriptionSubstrTokens, 'clasic', min_ngrams=>1);

/*---------------------------+
 | AlbumId | Description     |
 +---------------------------+
 | 2       | classical album |
 | 3       | last notes      |
 +---------------------------*/
```

The following query searches the column `  Description  ` for `  clasic albun  ` . As the `  DescriptionSubstrTokens  ` is tokenized by `  TOKENIZE_SUBSTRING  ` , the query is segmented into `  ['clasic', 'albun']  ` first, then n-gram tokens are generated from those words, producing the following: `  ['alb', 'asi', 'bun', 'cla', 'las', 'lbu', 'sic']  ` .

``` text
SELECT
  a.AlbumId, a.Description
FROM
  Albums a
WHERE
  SEARCH_NGRAMS(a.DescriptionSubstrTokens, 'clasic albun');

/*---------------------------+
 | AlbumId | Description     |
 +---------------------------+
 | 2       | classical album |
 | 1       | rock album      |
 +---------------------------*/
```

The following query searches the column `  Description  ` for `  l al  ` , but using the `  DescriptionNgramsTokens  ` this time. As the `  DescriptionNgramsTokens  ` is generated by `  TOKENIZE_NGRAMS  ` , there is no splitting into words before making n-gram tokens, so the query n-gram tokens are generated as the following: `  ['%20al', 'l%20a']  ` .

``` text
SELECT
  a.AlbumId, a.Description
FROM
  Albums a
WHERE
  SEARCH_NGRAMS(a.DescriptionNgramsTokens, 'l al');

/*---------------------------+
 | AlbumId | Description     |
 +---------------------------+
 | 2       | classical album |
 +---------------------------*/
```

## `     SEARCH_SUBSTRING    `

``` text
SEARCH_SUBSTRING(
  tokens,
  substring_query
  [, language_tag => value ]
  [, relative_search_type => value ]
)
```

**Description**

Returns `  TRUE  ` if a substring query matches tokens.

**Definitions**

  - `  tokens  ` : A `  TOKENLIST  ` value that contains a list of substring tokens. It must be a `  TOKENLIST  ` generated by either `  TOKENIZE_SUBSTRING  ` or by concatenating `  TOKENLIST  ` s from `  TOKENIZE_SUBSTRING  ` using `  TOKENLIST_CONCAT  ` .

  - `  substring_query  ` : A `  STRING  ` value that represents a substring query. `  substring_query  ` is first converted to lowercase to match `  tokens  ` that were converted to lowercase during tokenization.

  - `  language_tag  ` : A named argument with a `  STRING  ` value. The value contains an [IETF BCP 47 language tag](https://en.wikipedia.org/wiki/IETF_language_tag) . You can use this tag to specify the language for `  substring_query  ` . If the value for this argument is `  NULL  ` , this function doesn't use a specific language. If this argument isn't specified, `  NULL  ` is used by default.

  - `  relative_search_type  ` : A named argument with a `  STRING  ` value. The value refines the substring search result. To use a given `  relative_search_type  ` , the substring `  TOKENLIST  ` must have been generated with the corresponding type in its `  TOKENIZE_SUBSTRING  ` `  relative_search_types  ` argument. This function supports these relative search types:
    
      - `  phrase  ` : The substring query terms must appear adjacent to one another and in order in the tokenized value (the value that was tokenized to produce the `  tokens  ` argument).
    
      - `  value_prefix  ` : The substring query terms must be found at the start of tokenized value.
    
      - `  value_suffix  ` : The substring query terms must be found at the end of tokenized value.
    
      - `  word_prefix  ` : The substring query terms must be found at the start of a word in the tokenized value.
    
      - `  word_suffix  ` : The substring query terms must be found at the end of a word in the tokenized value.

**Details**

  - Returns `  TRUE  ` if `  tokens  ` is a match for `  substring_query  ` .
  - This function must reference a substring `  TOKENLIST  ` column in a table that is also indexed in a search index. To add a substring `  TOKENLIST  ` column to a table and to a search index, see the examples for this function.
  - This function returns `  NULL  ` when `  tokens  ` or `  substring_query  ` is `  NULL  ` .
  - This function can only be used in the `  WHERE  ` clause of a SQL query.

**Return type**

`  BOOL  `

**Examples**

The following examples reference a table called `  Albums  ` and a search index called `  AlbumsIndex  ` .

The `  Albums  ` table contains a column called `  DescriptionSubstrTokens  ` , which tokenizes the input added to the `  Description  ` column using `  TOKENIZE_SUBSTRING  ` , and then saves those substring tokens in the `  DescriptionSubstrTokens  ` column. Finally, `  AlbumsIndex  ` indexes `  DescriptionSubstrTokens  ` . Once `  DescriptionSubstrTokens  ` is indexed, it can be used with the `  SEARCH_SUBSTRING  ` function.

``` text
CREATE TABLE Albums (
  SingerId INT64 NOT NULL,
  AlbumId INT64 NOT NULL,
  Description STRING(MAX),
  DescriptionSubstrTokens TOKENLIST AS (TOKENIZE_SUBSTRING(Description, support_relative_search=>TRUE)) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(DescriptionSubstrTokens)
PARTITION BY SingerId;

INSERT INTO Albums (SingerId, AlbumId, Description) VALUES (1, 1, 'rock album');
INSERT INTO Albums (SingerId, AlbumId, Description) VALUES (1, 2, 'classical album');
```

The following query searches the column called `  Description  ` for a token called `  ssic  ` . If this token is found for singer ID `  1  ` , the matching rows are returned.

``` text
SELECT
  a.AlbumId, a.Description
FROM
  Albums a
WHERE
  a.SingerId = 1 AND SEARCH_SUBSTRING(a.DescriptionSubstrTokens, 'ssic');

/*---------------------------+
 | AlbumId | Description     |
 +---------------------------+
 | 2       | classical album |
 +---------------------------*/
```

The following query searches the column called `  Description  ` for a token called both `  lbu  ` and `  oc  ` . If these tokens are found for singer ID `  1  ` , the matching rows are returned.

``` text
SELECT
  a.AlbumId, a.Description
FROM
  Albums a
WHERE
  a.SingerId = 1 AND SEARCH_SUBSTRING(a.DescriptionSubstrTokens, 'lbu oc');

/*-----------------------+
 | AlbumId | Description |
 +-----------------------+
 | 1       | rock album  |
 +-----------------------*/
```

The following query searches the column called `  Description  ` for a token called `  al  ` at the start of a word. If this token is found for singer ID `  1  ` , the matching rows are returned.

``` text
SELECT
  a.AlbumId, a.Description
FROM
  Albums a
WHERE
  a.SingerId = 1 AND SEARCH_SUBSTRING(a.DescriptionSubstrTokens, 'al', relative_search_type=>'word_prefix');

/*---------------------------+
 | AlbumId | Description     |
 +---------------------------+
 | 2       | classical album |
 | 1       | rock album      |
 +---------------------------*/
```

The following query searches the column called `  Description  ` for a token called `  al  ` at the start of tokens. If this token is found for singer ID `  1  ` , the matching rows are returned. Because there are no matches, no rows are returned.

``` text
SELECT
  a.AlbumId, a.Description
FROM
  Albums a
WHERE
  a.SingerId = 1 AND SEARCH_SUBSTRING(a.DescriptionSubstrTokens, 'al', relative_search_type=>'value_prefix');

/*---------------------------+
 | AlbumId | Description     |
 +---------------------------+
 |         |                 |
 +---------------------------*/
```

## `     SNIPPET    `

``` text
SNIPPET(
  data_to_search,
  raw_search_query
  [, language_tag => value ]
  [, enhance_query => { TRUE | FALSE } ]
  [, max_snippet_width => value ]
  [, max_snippets => value ]
  [, content_type => { "text/plain" | "text/html" } ]
)
```

**Description**

Gets a list of snippets that match a full-text search query.

**Definitions**

  - `  data_to_search  ` : A `  STRING  ` value that represents the data to search over.

  - `  raw_search_query  ` : A `  STRING  ` value that represents the terms of a raw search query.

  - `  language_tag  ` : A named argument with a `  STRING  ` value. The value contains an [IETF BCP 47 language tag](https://en.wikipedia.org/wiki/IETF_language_tag) . You can use this tag to specify the language for `  raw_search_query  ` . If the value for this argument is `  NULL  ` , this function doesn't use a specific language. If this argument isn't specified, `  NULL  ` is used by default.

  - `  max_snippets  ` : A named argument with an `  INT64  ` value. The value represents the maximum number of output snippets to produce.

  - `  max_snippet_width  ` : A named argument with an `  INT64  ` value. The value represents the width of the output snippet. The width is measured by the estimated number of average proportional-width characters. For example, a wide character like `  'M'  ` uses more space than a narrow character like `  'i'  ` .

  - `  enhance_query  ` : A named argument with a `  BOOL  ` value. The value determines whether to enhance the search query. For example, if `  enhance_query  ` is enabled, a search query containing the term `  classic  ` can expand to include similar terms such as `  classical  ` . If the `  enhance_query  ` call times out, the search query proceeds without enhancement. However, if the query includes `  @{require_enhance_query=true} SELECT ...  ` , a timeout causes the entire query to fail instead. The default timeout for query enhancement is 500 ms, which you can override using a hint like `  @{enhance_query_timeout_ms=200} SELECT ...  ` .  
    Google is continuously improving the query enhancement algorithms. As a result, a query with `  enhance_query=>true  ` might yield slightly different results over time.
    
      - If `  TRUE  ` , the search query is enhanced to improve search quality.
    
      - If `  FALSE  ` (default), the search query isn't enhanced.

  - `  content_type  ` : A named argument with a `  STRING  ` value. Indicates the MIME type of `  data_to_search  ` . This can be:
    
      - `  "text/plain"  ` (default): `  data_to_search  ` contains plain text.
    
      - `  "text/html"  ` : `  data_to_search  ` contains HTML. The HTML tags are removed. HTML-escaped entities are replaced with their unescaped equivalents (for example, `  &lt;  ` becomes `  <  ` ).

**Details**

Each snippet contains a matching substring of the `  data_to_search  ` , and a list of highlights for the location of matching terms.

This function returns `  NULL  ` when `  data_to_search  ` or `  raw_search_query  ` is `  NULL  ` .

**Return type**

`  JSON  `

The `  JSON  ` value has this format and definitions:

``` text
{
  "snippets":[
    {
      "highlights":[
        {
          "begin": json_number,
          "end": json_number
        },
      ],
      "snippet": json_string,
      "source_begin": json_number,
      "source_end": json_number
    }
  ]
}
```

  - `  snippets  ` : A JSON object that contains snippets from `  data_to_search  ` . These are snippets of text for `  raw_search_query  ` from the provided `  data_to_search  ` argument.
  - `  highlights  ` : A JSON array that contains the position of each search term found in `  snippet  ` .
  - `  begin  ` : A JSON number that represents the position of a search term's first character in `  snippet  ` .
  - `  end  ` : A JSON number that represents the position of a search term's final character in `  snippet  ` .
  - `  snippet  ` : A JSON string that represents an individual snippet from `  snippets  ` .
  - `  source_begin  ` : A JSON number that represents the starting ordinal of the range within the `  data_to_search  ` argument that `  snippet  ` was sourced from. This range might not contain exactly the same text as the snippet itself. For example, HTML tags are removed from the snippet when `  content_type  ` is `  text/html  ` , and some types of punctuation and whitespace are either removed or normalized.
  - `  source_end  ` : A JSON number that represents the ordinal one past the end of the source range. Like `  source_begin  ` , can include whitespace or punctuation not present in the snippet itself.

**Examples**

The following query produces a single snippet, `  Rock albums rock.  ` with two highlighted positions for the matching raw search query term, `  rock  ` :

``` text
SELECT SNIPPET('Rock albums rock.', 'rock') AS Snippet;

/*--------------------------------------------------------------------------------------------------------------------------------------------------+
 | Snippet                                                                                                                                          |
 +--------------------------------------------------------------------------------------------------------------------------------------------------+
 | {"snippets":[{"highlights":[{"begin":"1","end":"5"},{"begin":"13","end":"17"}],"snippet":"Rock albums rock.","source_begin":1,"source_end":18}]} |
 +--------------------------------------------------------------------------------------------------------------------------------------------------*/
```

## `     TOKEN    `

``` text
TOKEN(value_to_tokenize)
```

**Description**

Constructs an exact match `  TOKENLIST  ` value by tokenizing a `  BYTE  ` or `  STRING  ` value verbatim to accelerate exact match expressions in SQL.

**Definitions**

  - `  value_to_tokenize  ` : A `  BYTE  ` , `  ARRAY<BYTE>  ` , `  STRING  ` or `  ARRAY<STRING>  ` value to tokenize for searching with exact match expressions.

**Details**

  - This function returns `  NULL  ` when `  value_to_tokenize  ` is `  NULL  ` .

**Return type**

`  TOKENLIST  `

**Examples**

The `  Albums  ` table contains a column called `  SingerNameToken  ` and `  SongTitlesToken  ` , which tokenizes the `  SingerName  ` and `  SongTitles  ` columns respectively using the `  TOKEN  ` function. Finally, `  AlbumsIndex  ` indexes `  SingerNameToken  ` and `  SongTitlesToken  ` , which makes it possible for Spanner to use the index to accelerate exact-match expressions in SQL.

``` text
CREATE TABLE Albums (
  SingerId INT64 NOT NULL,
  AlbumId INT64 NOT NULL,
  SingerName STRING(MAX),
  SingerNameToken TOKENLIST AS (TOKEN(SingerName)) HIDDEN,
  SongTitles ARRAY<STRING(MAX)>,
  SongTitlesToken TOKENLIST AS (TOKEN(SongTitles)) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(SingerNameToken, SongTitlesToken);

-- For example, the INSERT statement below generates SingerNameToken of
-- 'Catalina Smith', and SongTitlesToken of
-- ['Starting Again', 'The Second Title'].
INSERT INTO Albums (SingerId, AlbumId, SingerName, SongTitles)
  VALUES (1, 1, 'Catalina Smith', ['Starting Again', 'The Second Time']);
```

The following query finds the column `  SingerName  ` is equal to `  Catalina Smith  ` . The query optimizer could choose to accelerate the condition using `  AlbumsIndex  ` with `  SingerNameToken  ` . Optionally, the query can provide `  @{force_index = AlbumsIndex}  ` to force the optimizer to use `  AlbumsIndex  ` .

``` text
SELECT a.AlbumId
FROM Albums @{force_index = AlbumsIndex} a
WHERE a.SingerName = 'Catalina Smith';

/*---------+
 | AlbumId |
 +---------+
 | 1       |
 +---------*/
```

The following query is like the previous ones. However, this time the query searches for `  SongTitles  ` that contain the string `  Starting Again  ` . Array conditions should use `  ARRAY_INCLUDES  ` , `  ARRAY_INCLUDES_ANY  ` or `  ARRAY_INCLUDES_ALL  ` functions to be eligible for using a search index for acceleration.

``` text
SELECT a.AlbumId
FROM Albums a
WHERE ARRAY_INCLUDES(a.SongTitles, 'Starting Again');

/*---------+
 | AlbumId |
 +---------+
 | 1       |
 +---------*/
```

## `     TOKENIZE_BOOL    `

``` text
TOKENIZE_BOOL(value_to_tokenize)
```

**Description**

Constructs a boolean `  TOKENLIST  ` value by tokenizing a `  BOOL  ` value to accelerate boolean match expressions in SQL.

**Definitions**

  - `  value_to_tokenize  ` : A `  BOOL  ` or `  ARRAY<BOOL>  ` value to tokenize for boolean match.

**Details**

  - This function returns `  NULL  ` when `  value_to_tokenize  ` is `  NULL  ` .

**Return type**

`  TOKENLIST  `

**Examples**

The `  Albums  ` table contains a column called `  IsAwardedToken  ` , which tokenizes the `  IsAwarded  ` column using `  TOKENIZE_BOOL  ` function. Finally, `  AlbumsIndex  ` indexes `  IsAwardedToken  ` , which makes it possible for Spanner to use the index to accelerate boolean-match expressions in SQL.

``` text
CREATE TABLE Albums (
  SingerId INT64 NOT NULL,
  AlbumId INT64 NOT NULL,
  IsAwarded BOOL,
  IsAwardedToken TOKENLIST AS (TOKENIZE_BOOL(IsAwarded)) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(IsAwardedToken);

-- IsAwarded with TRUE generates IsAwardedToken with value 'y'.
INSERT INTO Albums (SingerId, AlbumId, IsAwarded) VALUES (1, 1, TRUE);

-- IsAwarded with FALSE generates IsAwardedToken with value 'n'.
INSERT INTO Albums (SingerId, AlbumId, IsAwarded) VALUES (1, 2, FALSE);

-- NULL IsAwarded generates IsAwardedToken with value NULL.
INSERT INTO Albums (SingerId, AlbumId) VALUES (1, 3);
```

The following query finds the column `  IsAwarded  ` is equal to `  TRUE  ` . The query optimizer could choose to accelerate the condition using `  AlbumsIndex  ` with `  IsAwardedToken  ` . Optionally, the query can provide `  @{force_index = AlbumsIndex}  ` to force the optimizer to use `  AlbumsIndex  ` .

``` text
SELECT a.AlbumId
FROM Albums @{force_index = AlbumsIndex} a
WHERE IsAwarded = TRUE;
```

## `     TOKENIZE_FULLTEXT    `

``` text
TOKENIZE_FULLTEXT(
  value_to_tokenize
  [, language_tag => value ]
  [, content_type => { "text/plain" | "text/html" } ]
  [, token_category => { "small" | "medium" | "large" | "title" } ]
  [, remove_diacritics => { TRUE | FALSE }]
)
```

**Description**

Constructs a full-text `  TOKENLIST  ` value by tokenizing text for full-text matching.

**Definitions**

  - `  value_to_tokenize  ` : A `  STRING  ` or `  ARRAY<STRING>  ` value to tokenize for full-text search.

  - `  language_tag  ` : A named argument with a `  STRING  ` value. The value contains an [IETF BCP 47 language tag](https://en.wikipedia.org/wiki/IETF_language_tag) . You can use this tag to specify the language for `  value_to_tokenize  ` . If the value for this argument is `  NULL  ` , this function doesn't use a specific language. If this argument isn't specified, `  NULL  ` is used by default.

  - `  content_type  ` : A named argument with a `  STRING  ` value. Indicates the MIME type of `  value  ` . This can be:
    
      - `  "text/plain"  ` (default): `  value_to_tokenize  ` contains plain text. All tokens are assigned to the *small* token category.
    
      - `  "text/html"  ` : `  value_to_tokenize  ` contains HTML. The HTML tags are removed. HTML-escaped entities are replaced with their unescaped equivalents (for example, `  &lt;  ` becomes `  <  ` ). A token category is assigned to each token depending on its prominence in the HTML. For example, bolded text or text in a `  <h1>  ` tag might have higher prominence than normal text and thus might be placed into a different token category.
        
        We use token categories during scoring to boost the weight of high-prominence tokens.

  - `  token_category  ` : A named argument with a `  STRING  ` value. Sets or overrides the token importance signals detected by the tokenizer and used by the scorer. Useful for cases where two or more `  TOKENLIST  ` s are combined with `  TOKENLIST_CONCAT  ` and one of the input columns is known to have higher or lower than usual importance.
    
    Allowed values:
    
      - `  "small"  ` : The category with the lowest importance.
      - `  "medium"  ` : The category with the second lowest importance.
      - `  "large"  ` : The category with the second highest importance.
      - `  "title"  ` : The category with the highest importance.

  - `  remove_diacritics  ` : A named argument with a `  BOOL  ` value. If `  TRUE  ` , the diacritics are removed from `  value_to_tokenize  ` before indexing. This is useful when you want to ignore diacritics when searching (full-text, substring, or ngram). When a search query is called on a `  TOKENLIST  ` value with `  remove_diacritics  ` set as `  TRUE  ` , the diacritics are also removed at query time from the search queries.

**Details**

  - This function returns `  NULL  ` when `  value_to_tokenize  ` is `  NULL  ` .

**Return type**

`  TOKENLIST  `

**Examples**

In the following example, a `  TOKENLIST  ` column is created using the `  TOKENIZE_FULLTEXT  ` function:

``` text
CREATE TABLE Albums (
  SingerId INT64 NOT NULL,
  AlbumId INT64 NOT NULL,
  Title STRING(MAX),
  Description STRING(MAX),
  DescriptionTokens TOKENLIST AS (TOKENIZE_FULLTEXT(Description)) HIDDEN,
  TitleTokens TOKENLIST AS (
    TOKENIZE_FULLTEXT(Title, token_category=>"title")) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

-- DescriptionTokens is generated from the Description value, using the
-- TOKENIZE_FULLTEXT function. For example, the following INSERT statement
-- generates DescriptionTokens with the tokens ['rock', 'album']. TitleTokens
-- will contain ['abbey', 'road'] and these tokens will be assigned to the
-- "title" token category.
INSERT INTO Albums (SingerId, AlbumId, Description) VALUES (1, 1, 'rock album');

-- Capitalization and delimiters are removed during tokenization. For example,
-- the following INSERT statement generates DescriptionTokens with the tokens
-- ['classical', 'albums'].
INSERT INTO Albums (SingerId, AlbumId, Description) VALUES (1, 1, 'Classical, Albums.');
```

To query a full-text `  TOKENLIST  ` column, see the [SEARCH](#search) function.

## `     TOKENIZE_JSON    `

``` text
TOKENIZE_JSON(value_to_tokenize)
```

**Description**

Constructs a JSON `  TOKENLIST  ` value by tokenizing a `  JSON  ` value to accelerate JSON predicate matching in SQL.

**Definitions**

  - `  value_to_tokenize  ` : A `  JSON  ` value to tokenize for JSON predicate matching.

**Details**

  - This function returns `  NULL  ` when `  value_to_tokenize  ` is `  NULL  ` .

**Return type**

`  TOKENLIST  `

**Examples**

The `  Albums  ` table contains a column called `  MetadataTokens  ` , which tokenizes the `  Metadata  ` column using the `  TOKENIZE_JSON  ` function. `  AlbumsIndex  ` indexes `  MetadataToken  ` , which makes it possible for Spanner to use the index to accelerate JSON predicate expressions in SQL.

``` text
CREATE TABLE Albums (
  SingerId INT64 NOT NULL,
  AlbumId INT64 NOT NULL,
  Metadata JSON,
  MetadataTokens TOKENLIST AS (TOKENIZE_JSON(Metadata)) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(MetadataTokens);

-- Albums can be stored with varying metadata.
INSERT INTO Albums (SingerId, AlbumId, Metadata)
  VALUES (1, 1, JSON '{"AvailableFormats": ["vinyl", "cd"]}'),
         (1, 2, JSON '{"ReissueDate": "1999-07-13", "MultiDiscCount": 2}'),
         (1, 3, JSON '{"RegionalReleases": [{"Region": "Japan", "ReleaseDate": "2025-01-05"}]}');
```

The following queries perform containment and existence checks on the `  Metadata  ` column. The query optimizer might choose to accelerate these conditions using `  AlbumsIndex  ` and `  MetadataTokens  ` .

``` text
-- Query for albums available on vinyl.
SELECT a.AlbumId
FROM Albums a
WHERE JSON_CONTAINS(a.Metadata, JSON '{"AvailableFormats": ["vinyl"]}');

/*---------+
 | AlbumId |
 +---------+
 | 1       |
 +---------*/

-- Query for albums with a regional release in Japan.
SELECT a.AlbumId
FROM Albums a
WHERE JSON_CONTAINS(a.Metadata, JSON '{"RegionalReleases": [{"Region": "Japan"}]}');

/*---------+
 | AlbumId |
 +---------+
 | 3       |
 +---------*/

-- Query for reissued albums (those with a reissue date).
SELECT a.AlbumId
FROM Albums a
WHERE a.Metadata.ReissueDate IS NOT NULL;

/*---------+
 | AlbumId |
 +---------+
 | 2       |
 +---------*/
```

## `     TOKENIZE_NGRAMS    `

``` text
TOKENIZE_NGRAMS(
  value_to_tokenize
  [, ngram_size_min => value ]
  [, ngram_size_max => value ]
  [, remove_diacritics => { TRUE | FALSE } ]
)
```

**Description**

Constructs an n-gram `  TOKENLIST  ` value by tokenizing text for n-gram matching.

**Definitions**

  - `  value_to_tokenize  ` : A `  STRING  ` or `  ARRAY<STRING>  ` value to tokenize for n-gram search.

  - `  ngram_size_min  ` : A named argument with an `  INT64  ` value. The value is the minimum length of the n-gram tokens to generate. The default value for this argument is `  1  ` . This argument must be less than or equal to `  ngram_size_max  ` .
    
    Increasing `  ngram_size_min  ` can reduce write overhead and index size by generating fewer tokens. However, since n-gram tokens shorter than `  ngram_size_min  ` are not generated, n-gram search queries that require those tokens are not able to find any matches.
    
    We recommend tuning `  ngram_size_min  ` only when the developer controls the queries and can ensure that the minimum query length is at least `  ngram_size_min  ` .

  - `  ngram_size_max  ` : A named argument with an `  INT64  ` value. The value is the maximum size of each n-gram token to generate. Setting a higher `  ngram_size_max  ` can lead to better retrieval performance by reducing the number of irrelevant records that share common n-grams. However, a larger difference between `  ngram_size_min  ` and `  ngram_size_max  ` can substantially increase index sizes and write costs.
    
    When using the resulting `  TOKENLIST  ` with `  SEARCH_NGRAMS  ` , the `  ngram_size_max  ` parameter also determines the length of n-grams generated for the `  ngrams_query  ` parameter of `  SEARCH_NGRAMS  ` . Opting for a shorter n-gram length in your query yields a higher number of matches, but can also introduce irrelevant results.
    
    The default value for this argument is `  4  ` . However, when using the resulting `  TOKENLIST  ` with the `  SEARCH_NGRAMS  ` function, `  ngram_size_max  ` of 3 can be a good starting point for matching common typographical errors. Further fine-tuning can help with specific fuzzy search queries and data patterns.

  - `  remove_diacritics  ` : A named argument with a `  BOOL  ` value. If `  TRUE  ` , the diacritics are removed from `  value_to_tokenize  ` before indexing. This is useful when you want to ignore diacritics when searching (full-text, substring, or ngram). When a search query is called on a `  TOKENLIST  ` value with `  remove_diacritics  ` set as `  TRUE  ` , the diacritics are also removed at query time from the search queries.

**Details**

  - This function returns `  NULL  ` when `  value_to_tokenize  ` is `  NULL  ` .

**Return type**

`  TOKENLIST  `

**Examples**

In the following example, a `  TOKENLIST  ` column is created using the `  TOKENIZE_NGRAMS  ` function. The `  INSERT  ` generates a `  TOKENLIST  ` which contains two sets of tokens. First, the whole string is broken up into n-grams with a length in the range `  [ngram_size_min, ngram_size_max-1]  ` . Capitalization and whitespace are preserved in the n-grams. These n-grams are placed in the first position in the tokenlist.

`  [" ", " M", " Me", "vy ", "y ", "y M", H, He, Hea, Heav, ...], ...  `

Second, any n-grams with length equal to `  ngram_size_max  ` are stored in sequence, with the first of these in the same position as the smaller n-grams. (In this example, the `  Heav  ` token is in the first position.)

`  ..., eavy, "avy ", "vy M", "y Me", " Met", Meta, etal  `

``` text
CREATE TABLE Albums (
  AlbumId INT64 NOT NULL,
  Description STRING(MAX),
  DescriptionNgramTokens TOKENLIST AS (TOKENIZE_NGRAMS(Description)) HIDDEN
) PRIMARY KEY (AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(DescriptionNgramTokens);

INSERT INTO Albums (AlbumId, Description) VALUES (1, 'Heavy Metal');
```

To query an n-gram `  TOKENLIST  ` column, see the [SEARCH\_NGRAMS](#search_ngrams) function.

## `     TOKENIZE_NUMBER    `

``` text
TOKENIZE_NUMBER(
  value_to_tokenize,
  [, comparison_type => { "all" | "equality" } ]
  [, algorithm => { "logtree" | "prefixtree" | "floatingpoint" } ]
  [, min => value ]
  [, max => value ]
  [, granularity => value ]
  [, tree_base => value ]
  [, precision => value ]
)
```

**Description**

Constructs a numeric `  TOKENLIST  ` value by tokenizing numeric values to accelerate numeric comparison expressions in SQL.

**Definitions**

  - `  value_to_tokenize  ` : An `  INT64  ` , `  FLOAT32  ` , `  FLOAT64  ` or `  ARRAY  ` of these types to tokenize for numeric comparison expressions.

  - `  comparison_type  ` : A named argument with a `  STRING  ` value. The value represents the type of comparison to use for numeric expressions. Set to `  equality  ` to save space if equality is only required comparison. Default is `  all  ` .

  - `  algorithm  ` : A named argument with a `  STRING  ` value. The value indicates the indexing algorithm to use. Supported algorithms are limited, depending on the type of value being indexed. The default is `  logtree  ` . `  FLOAT32  ` or `  FLOAT64  ` must not use default. They should specify the algorithm and must also use `  min  ` and `  max  ` when using the `  logtree  ` or `  prefixtree  ` algorithms.
    
      - `  logtree  ` : Use for indexing uniformly distributed data. `  min  ` , `  max  ` , and `  granularity  ` must be specified if `  value_to_tokenize  ` is `  FLOAT32  ` or `  FLOAT64  ` .
      - `  prefixtree  ` : Use when indexing exponentially distributed data and when query predicate is of the form " `  @param > number  ` " or " `  @param >= number  ` " (ranges without an upper bound). Compared to `  logtree  ` , this algorithm generates fewer *index* tokens for small numbers. For queries where the `  WHERE  ` clause contains the predicate previously described, `  prefixtree  ` generates fewer *query* tokens, which can improve performance. `  min  ` , `  max  ` , and `  granularity  ` must be specified if `  value_to_tokenize  ` is `  FLOAT32  ` or `  FLOAT64  ` .
      - `  floatingpoint  ` : Use for indexing `  FLOAT32  ` or `  FLOAT64  ` values where the indexed data and queries often contain fractions. When tokenizing `  FLOAT32  ` or `  FLOAT64  ` using `  logtree  ` or `  prefixtree  ` , `  TOKENIZE_NUMBER  ` might lose precision as the count of `  granularity  ` buckets in the `  min  ` to `  max  ` range approaches the maximum resolution of floating point numbers. This can make queries less efficient, but it doesn't cause incorrect behavior. This loss of precision doesn't happen with the `  floatingpoint  ` algorithm if the `  precision  ` argument is set high enough. However, the `  floatingpoint  ` algorithm generates more index tokens when `  precision  ` is set to a larger value.

  - `  min  ` : A named argument with the same type as `  value_to_tokenize  ` . Values less than `  min  ` are indexed in the same index bucket. This will not cause incorrect results, but may cause significant over-retrieval for queries with a range that includes values lesser than `  min  ` . Don't use `  min  ` when `  comparison_type  ` is `  equality  ` .

  - `  max  ` : A named argument with the same type as `  value_to_tokenize  ` . Values greater than `  max  ` are indexed in the same index bucket. This doesn't cause incorrect results, but might cause significant over-retrieval for queries with a range that includes values greater than the `  max  ` . Don't use `  max  ` when `  comparison_type  ` is `  equality  ` .

  - `  granularity  ` : A named argument with the same type as `  value_to_tokenize  ` . The value represents the width of each indexing bucket. Values in the same bucket are indexed together, so larger buckets are more storage efficient, but may cause over-retrieval, causing high latency during query execution. `  granularity  ` is only allowed when `  algorithm  ` is `  logtree  ` or `  prefixtree  ` .

  - `  tree_base  ` : A named argument with an `  INT64  ` value. The value is the numerical base of a tree for tree-based algorithms.
    
    For example, the value of `  2  ` means that each tree token represents some power-of-two number of buckets. In the case of a value indexed in the 1024th bucket, there is a token for \[1024,1024\], then a token for \[1024,1025\], then a token for \[1024, 1027\], and so on.
    
    Increasing `  tree_base  ` reduces the required number of index tokens and increases the required number of query tokens.
    
    The default value is 2. `  tree_base  ` is only allowed when `  algorithm  ` is `  logtree  ` or `  prefixtree  ` .

  - `  precision  ` : A named argument with an `  INT64  ` value. Reducing the precision reduces the number of index tokens, but increases over-retrieval when queries specify ranges with a high number of significant digits. The default value is 15. `  precision  ` is only allowed when `  algorithm  ` is `  floatingpoint  ` .

**Details**

  - This function returns `  NULL  ` when `  value_to_tokenize  ` is `  NULL  ` .
  - The `  tree_base  ` parameter controls the width of each tree bucket in the `  logtree  ` and `  prefixtree  ` algorithms. Both algorithms generate tokens representing nodes in a `  base  ` -ary tree where the width of a node is `  base  ` <sup>`  distance_from_leaf  `</sup> . The algorithms differ in that `  prefixtree  ` omits some of the tree nodes in favor of greater-than tokens that accelerate greater-than queries. When a larger base is selected, fewer index tokens are generated. However, larger `  base  ` values increase the maximum number of query tokens required.
  - Numbers that fall outside of the `  [min, max]  ` range are all indexed into two buckets: one for all numbers less than `  min  ` , and the other for all numbers greater than `  max  ` . This might cause significant over-retrieval (retrieval of too many candidate results) when the range requested by the query also includes numbers outside of the range. For this reason, set `  min  ` and `  max  ` to the narrowest possible values that encompass all input numbers. Like all tokenization configurations, changing the `  min  ` and `  max  ` values requires a rebuild of the numeric index, so leave room to grow if the final domain of a column isn't known. The problem of over-retrieval isn't a correctness problem as all potential matches are checked against non-bucketized numbers at the end of the search process; it's only a potential efficiency issue.
  - The `  granularity  ` argument controls the rate of downsampling that's applied to numbers before they are indexed in the tree-based algorithms. Before each number is tokenized, it's sorted into buckets with a width equal to `  granularity  ` . All the numbers in the same `  granularity  ` bucket get the same tokens. This means that over-retrieval might occur if the granularity value is set to anything other than 1 for integral numbers. Over retrieval is always possible for `  FLOAT64  ` numbers. It also means that if numeric values change by a small amount, most of their tokens don't need to be reindexed. Using a `  granularity  ` higher than 1 also reduces the number of tokens that the algorithm needs to generate, but the effect is less significant than the effect of increasing the `  base  ` . Therefore, we recommend that 'granularity' is set to 1.

**Return type**

`  TOKENLIST  `

**Examples**

The `  Albums  ` table contains a column called the `  RatingTokens  ` , which tokenizes the `  Rating  ` column using the `  TOKENIZE_NUMBER  ` function. Finally, `  AlbumsIndex  ` indexes `  RatingTokens  ` , which makes it possible for Spanner to use the index to accelerate numeric comparison expressions in SQL.

``` text
CREATE TABLE Albums (
  SingerId INT64 NOT NULL,
  AlbumId INT64 NOT NULL,
  Rating INT64,
  RatingTokens TOKENLIST AS (TOKENIZE_NUMBER(Rating)) HIDDEN,
  TrackRating ARRAY<INT64>,
  TrackRatingTokens TOKENLIST AS (TOKENIZE_NUMBER(TrackRating)) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(RatingTokens, TrackRatingTokens);

-- RatingTokens and TrackRatingTokens are generated from Rating and TrackRating
-- values, respectively, using the TOKENIZE_NUMBER function.
INSERT INTO Albums (SingerId, AlbumId, Rating, TrackRating) VALUES (1, 1, 2, [2, 3]);
INSERT INTO Albums (SingerId, AlbumId, Rating, TrackRating) VALUES (1, 2, 5, [3, 5]);
```

The following query finds rows in which the column `  Rating  ` is equal to `  5  ` . The query optimizer might choose to accelerate the condition using `  AlbumsIndex  ` with `  RatingTokens  ` . Optionally, the query can provide `  @{force_index = AlbumsIndex}  ` to force the optimizer to use `  AlbumsIndex  ` .

``` text
SELECT a.AlbumId
FROM Albums @{force_index = AlbumsIndex} a
WHERE a.Rating = 5;

/*---------+
 | AlbumId |
 +---------+
 | 2       |
 +---------*/
```

The following query is like the previous one. However, the condition is on the array column of `  TrackRating  ` this time. Array conditions should use `  ARRAY_INCLUDES  ` , `  ARRAY_INCLUDES_ANY  ` or `  ARRAY_INCLUDES_ALL  ` functions to be eligible for using a search index for acceleration.

``` text
SELECT a.AlbumId
FROM Albums a
WHERE ARRAY_INCLUDES_ALL(a.TrackRating, [2, 3]);

/*---------+
 | AlbumId |
 +---------+
 | 1       |
 +---------*/

SELECT a.AlbumId
FROM Albums a
WHERE ARRAY_INCLUDES_ANY(a.TrackRating, [3, 4, 5]);

/*---------+
 | AlbumId |
 +---------+
 | 1       |
 | 2       |
 +---------*/
```

The following query is like the previous ones. However, the condition is range this time. This query can also be accelerated, as default `  comparison_type  ` is `  all  ` which covers both `  equality  ` and `  range  ` comparisons.

``` text
SELECT a.AlbumId
FROM Albums a
WHERE a.Rating >= 2;

/*---------+
 | AlbumId |
 +---------+
 | 1       |
 | 2       |
 +---------*/
```

## `     TOKENIZE_SUBSTRING    `

``` text
TOKENIZE_SUBSTRING(
  value_to_tokenize
  [, language_tag => value ]
  [, ngram_size_min => value ]
  [, ngram_size_max => value ]
  [, relative_search_types => value ]
  [, content_type => { "text/plain" | "text/html" } ]
  [, short_tokens_only_for_anchors => {TRUE | FALSE } ]
  [, remove_diacritics => { TRUE | FALSE } ]
)
```

**Description**

Constructs a substring `  TOKENLIST  ` value by tokenizing text for substring matching.

**Definitions**

  - `  value_to_tokenize  ` : A `  STRING  ` or `  ARRAY<STRING>  ` value to tokenize for substring search. `  value_to_tokenize  ` is split into lower-cased words first, then n-gram tokens are generated from each word.

  - `  language_tag  ` : A named argument with a `  STRING  ` value. The value contains an [IETF BCP 47 language tag](https://en.wikipedia.org/wiki/IETF_language_tag) . You can use this tag to specify the language for `  value_to_tokenize  ` . If the value for this argument is `  NULL  ` , this function doesn't use a specific language. If this argument isn't specified, `  NULL  ` is used by default.

  - `  relative_search_types  ` : A named argument with an `  ARRAY<STRING>  ` value. The value determines which `  TOKENIZE_SUBSTRING  ` relative search types are supported. See the [`  SEARCH_SUBSTRING  `](#search_substring) function for a list of the different relative search types.
    
    In addition to the relative search types from the `  SEARCH_SUBSTRING  ` function, the `  TOKENIZE_SUBSTRING  ` function accepts a special flag, `  all  ` , which means that all relative search types are supported.
    
    If this argument isn't used, then no relative search tokens are generated for the resulting `  TOKENLIST  ` value.
    
    Setting this value causes extra *anchor* tokens to be generated to enable relative searches. A given relative search type can only be used in a query if that type, or `  all  ` , is present in the `  relative_search_types  ` argument. By default, `  relative_search_types  ` is empty.

  - `  content_type  ` : A named argument with a `  STRING  ` value. Indicates the MIME type of `  value  ` . This can be:
    
      - `  "text/plain"  ` (default): `  value_to_tokenize  ` contains plain text. All tokens are assigned to the *small* token category.
    
      - `  "text/html"  ` : `  value_to_tokenize  ` contains HTML. The HTML tags are removed. HTML-escaped entities are replaced with their unescaped equivalents (for example, `  &lt;  ` becomes `  <  ` ). A token category is assigned to each token depending on its prominence in the HTML. For example, bolded text or text in a `  <h1>  ` tag might have higher prominence than normal text and thus might be placed into a different token category.
        
        We use token categories during scoring to boost the weight of high-prominence tokens.

  - `  ngram_size_min  ` : A named argument with an `  INT64  ` value. The value is the minimum length of the n-gram tokens to generate. The default value for this argument is `  1  ` . This argument must be less than or equal to `  ngram_size_max  ` .
    
    While partial-word n-grams shorter than `  ngram_size_min  ` are not generated, tokens for whole words that are shorter than `  ngram_size_min  ` are. This lets `  SEARCH_SUBSTRING  ` match values containing such words, but only if the query text contains these tokens as words.
    
    Increasing `  ngram_size_min  ` can reduce write overhead and index size by generating fewer tokens. However, since n-gram tokens shorter than `  ngram_size_min  ` are not generated except for whole words, substring search queries that require those tokens are not able to find any matches.
    
    We recommend tuning `  ngram_size_min  ` only when the developer controls the queries and can ensure that the minimum query length is at least `  ngram_size_min  ` .

  - `  ngram_size_max  ` : A named argument with an `  INT64  ` value. The value is the maximum size of each n-gram token to generate. Setting a higher `  ngram_size_max  ` can lead to better retrieval performance by reducing the number of irrelevant records that share common n-grams. However, a larger difference between `  ngram_size_min  ` and `  ngram_size_max  ` can substantially increase index sizes and write costs.
    
    When using the resulting `  TOKENLIST  ` with `  SEARCH_NGRAMS  ` , the `  ngram_size_max  ` parameter also determines the length of n-grams generated for the `  ngrams_query  ` parameter of `  SEARCH_NGRAMS  ` . Opting for a shorter n-gram length in your query yields a higher number of matches, but can also introduce irrelevant results.
    
    The default value for this argument is `  4  ` . However, when using the resulting `  TOKENLIST  ` with the `  SEARCH_NGRAMS  ` function, `  ngram_size_max  ` of 3 can be a good starting point for matching common typographical errors. Further fine-tuning can help with specific fuzzy search queries and data patterns.

  - `  short_tokens_only_for_anchors  ` : A named argument with a `  BOOL  ` value. If true, the `  TOKENLIST  ` emitted by this function doesn't contain short n-grams — those with sizes less than `  ngram_size_max  ` — except when those n-grams are part of one of the anchors used to support the prefix and suffix `  relative_search_types  ` settings. The default value is `  FALSE  ` .
    
    Setting this to `  TRUE  ` can reduce the number of n-grams generated. However, it causes `  SEARCH_SUBSTRING  ` to return `  FALSE  ` for short query terms when `  relative_search_types  ` isn't one of the prefix or suffix modes. Therefore, we recommend setting this only when `  relative_search_types  ` is always set to a prefix or suffix mode.

  - `  remove_diacritics  ` : A named argument with a `  BOOL  ` value. If `  TRUE  ` , the diacritics are removed from `  value_to_tokenize  ` before indexing. This is useful when you want to ignore diacritics when searching (full-text, substring, or ngram). When a search query is called on a `  TOKENLIST  ` value with `  remove_diacritics  ` set as `  TRUE  ` , the diacritics are also removed at query time from the search queries.

**Details**

  - This function returns `  NULL  ` when `  value_to_tokenize  ` is `  NULL  ` .

**Return type**

`  TOKENLIST  `

**Example**

In the following example, a `  TOKENLIST  ` column is created using the `  TOKENIZE_SUBSTRING  ` function. The `  INSERT  ` generates a `  TOKENLIST  ` which contains two sets of tokens. First, each word is broken up into lower-cased n-grams with a length in the range `  [ngram_size_min, ngram_size_max-1]  ` , and any whole words with a length shorter than that `  ngram_size_max  ` . All of these tokens are placed in the first position in the tokenlist.

`  [a, al, av, avy, e, ea, eav, et, eta, h, he, hea, ...], ...  `

Second, any n-grams with length equal to `  ngram_size_max  ` are stored in subsequent positions. These tokens are used when searching for words larger than the maximum n-gram size.

`  ..., heav, eavy, <gap(1)>, meta, etal  `

``` text
CREATE TABLE Albums (
  SingerId INT64 NOT NULL,
  AlbumId INT64 NOT NULL,
  Description STRING(MAX),
  DescriptionSubstrTokens TOKENLIST
    AS (TOKENIZE_SUBSTRING(Description, ngram_size_min=>1, ngram_size_max=>4)) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

INSERT INTO Albums (SingerId, AlbumId, Description)
  VALUES (1, 1, 'Heavy Metal');
```

To query a substring `  TOKENLIST  ` column, see the [SEARCH\_SUBSTRING](#search_substring) or [SEARCH\_NGRAMS](#search_ngrams) function.

## `     TOKENLIST_CONCAT    `

``` text
TOKENLIST_CONCAT(value1 [, ...])
```

**Description**

Constructs a `  TOKENLIST  ` value by concatenating one or more `  TOKENLIST  ` values.

**Details**

  - This function only takes TOKENLIST generated by `  TOKENIZE_FULLTEXT  ` or `  TOKENIZE_SUBSTRING  ` .
  - All the `  TOKENLIST  ` args must be generated by the same tokenization functions.
  - This function returns `  NULL  ` when an array of TOKENLIST is `  NULL  ` .
  - This function treats the `  NULL  ` element in the array as an empty `  TOKENLIST  ` .

**Return type**

`  TOKENLIST  `

**Examples**

In the following example, full-text `  TOKENLIST  ` columns are created using the `  TOKENIZE_FULLTEXT  ` function, then another full-text `  TOKENLIST  ` column is created using the `  TOKENLIST_CONCAT  ` function:

``` text
CREATE TABLE Albums (
  SingerId INT64 NOT NULL,
  AlbumId INT64 NOT NULL,
  SingerName STRING(MAX),
  SingerNameTokens TOKENLIST AS (TOKENIZE_FULLTEXT(SingerName)) HIDDEN,
  AlbumName STRING(MAX),
  AlbumNameTokens TOKENLIST AS (TOKENIZE_FULLTEXT(AlbumName)) HIDDEN,
  SingerOrAlbumNameTokens TOKENLIST AS (TOKENLIST_CONCAT([SingerNameTokens, AlbumNameTokens])) HIDDEN
) PRIMARY KEY (SingerId, AlbumId);

CREATE SEARCH INDEX AlbumsIndex ON Albums(SingerNameTokens, AlbumNameTokens, SingerOrAlbumNameTokens);

-- The INSERT statement below generates SingerOrAlbumNameTokens by concatenating
-- all the tokens in SingerNameTokens and AlbumNameTokens.
INSERT INTO Albums (SingerId, AlbumId, SingerName, AlbumName) VALUES (1, 1, 'Alice Trentor', 'Go Go Go');
INSERT INTO Albums (SingerId, AlbumId, SingerName, AlbumName) VALUES (2, 1, 'Catalina Smith', 'Alice Wonderland');
```

The following query searches for a token `  alice  ` in the `  SingerOrAlbumNameColumnTokens  ` . The rows that match `  alice  ` in either `  SingerNameTokens  ` or `  AlbumNameTokens  ` are returned.

``` text
SELECT a.SingerId, a.AlbumId
FROM Albums a
WHERE SEARCH(a.SingerOrAlbumNameTokens, 'alice');

/*--------------------+
 | SingerId | AlbumId |
 +--------------------+
 | 2        | 1       |
 | 1        | 1       |
 +--------------------*/
```

The following query is like the previous one. However, `  TOKENLIST_CONCAT  ` is called directly inside of a `  SEARCH  ` function this time.

``` text
SELECT a.SingerId, a.AlbumId
FROM Albums a
WHERE SEARCH(TOKENLIST_CONCAT([a.SingerNameTokens, a.AlbumNameTokens]), 'alice');

/*--------------------+
 | SingerId | AlbumId |
 +--------------------+
 | 2        | 1       |
 | 1        | 1       |
 +--------------------*/
```
