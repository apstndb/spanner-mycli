GoogleSQL for Spanner supports the following machine learning (ML) functions.

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
<td><a href="/spanner/docs/reference/standard-sql/ml-functions#mlpredict"><code dir="ltr" translate="no">        ML.PREDICT       </code></a></td>
<td>Apply ML computations defined by a model to each row of an input relation.</td>
</tr>
</tbody>
</table>

## `     ML.PREDICT    `

``` text
ML.PREDICT(input_model, input_relation[, model_parameters])

input_model:
  MODEL model_name

input_relation:
  { input_table | input_subquery }

input_table:
  TABLE table_name

model_parameters:
  STRUCT(parameter_value AS parameter_name[, ...])
```

**Description**

`  ML.PREDICT  ` is a table-valued function that helps to access registered machine learning (ML) models and use them to generate ML predictions. This function applies ML computations defined by a model to each row of an input relation, and returns the results of those predictions. Additionally, you can use `  ML.PREDICT  ` to perform vector search. When you use `  ML.PREDICT  ` for vector search, it converts your natural language query text into an embedding.

**Note:** Make sure that Spanner has access to the referenced Vertex AI endpoint as described in [Model endpoint access control](/spanner/docs/reference/standard-sql/data-definition-language#create_model_permissions) .

**Supported Argument Types**

  - `  input_model  ` : The model to use for predictions. Replace `  model_name  ` with the name of the model. To create a model, see [`  CREATE_MODEL  `](/spanner/docs/reference/standard-sql/data-definition-language#create_model) .
  - `  input_relation  ` : A table or subquery upon which to apply ML computations. The set of columns of the input relation must include all input columns of the input model; otherwise, the input won't have enough data to generate predictions and the query won't compile. Additionally, the set can also include arbitrary pass-through columns that will be included in the output. The order of the columns in the input relation doesn't matter. The columns of the input relation and model must be coercible.
  - `  input_table  ` : The table containing the input data for predictions, for example, a set of features. Replace `  table_name  ` with the name of the table.
  - `  input_subquery  ` : The subquery that's used to generate the prediction input data.
  - `  model_parameters  ` : A `  STRUCT  ` value that contains parameters supported by `  model_name  ` . These parameters are passed to the model inference.

**Return Type**

A table with the following columns:

  - Model outputs
  - Pass-through columns from the input relation

**Note:** If a column of the input relation has the same name as one of the output columns, the value of the output column is returned.

**Examples**

The examples in this section reference a model called `  DiamondAppraise  ` and an input table called `  Diamonds  ` with the following columns:

  - `  DiamondAppraise  ` model:
    
    <table>
    <thead>
    <tr class="header">
    <th>Input columns</th>
    <th>Output columns</th>
    </tr>
    </thead>
    <tbody>
    <tr class="odd">
    <td><code dir="ltr" translate="no">         value FLOAT64        </code></td>
    <td><code dir="ltr" translate="no">         value FLOAT64        </code></td>
    </tr>
    <tr class="even">
    <td><code dir="ltr" translate="no">         carat FLOAT64        </code></td>
    <td><code dir="ltr" translate="no">         lower_bound FLOAT64        </code></td>
    </tr>
    <tr class="odd">
    <td><code dir="ltr" translate="no">         cut STRING        </code></td>
    <td><code dir="ltr" translate="no">         upper_bound FLOAT64        </code></td>
    </tr>
    <tr class="even">
    <td><code dir="ltr" translate="no">         color STRING(1)        </code></td>
    <td></td>
    </tr>
    </tbody>
    </table>

  - `  Diamonds  ` table:
    
    <table>
    <thead>
    <tr class="header">
    <th>Columns</th>
    </tr>
    </thead>
    <tbody>
    <tr class="odd">
    <td><code dir="ltr" translate="no">         Id INT64        </code></td>
    </tr>
    <tr class="even">
    <td><code dir="ltr" translate="no">         Carat FLOAT64        </code></td>
    </tr>
    <tr class="odd">
    <td><code dir="ltr" translate="no">         Cut STRING        </code></td>
    </tr>
    <tr class="even">
    <td><code dir="ltr" translate="no">         Color STRING        </code></td>
    </tr>
    </tbody>
    </table>

The following query predicts the value of a diamond based on the diamond's carat, cut, and color.

``` text
SELECT id, color, value
FROM ML.PREDICT(MODEL DiamondAppraise, TABLE Diamonds);

+----+-------+-------+
| id | color | value |
+----+-------+-------+
| 1  | I     | 280   |
| 2  | G     | 447   |
+----+-------+-------+
```

You can include model-specific parameters. For example, in the following query, the `  maxOutputTokens  ` parameter specifies that `  output  ` , the model inference, can contain 10 or fewer tokens. This query succeeds because the model `  TextBison  ` contains a parameter called `  maxOutputTokens  ` .

``` text
SELECT prompt, output
FROM ML.PREDICT(
  MODEL TextBison,
  (SELECT "Is 13 prime?" as prompt), STRUCT(10 AS maxOutputTokens));

+----------------+---------------------+
| prompt         | output             |
+----------------+---------------------+
| "Is 13 prime?" | "Yes, 13 is prime." |
+----------------+---------------------+
```

The following example generates an embedding for a natural language query. The example then uses that embedding to find the most similar entries in a database that are indexed by vector embeddings.

``` text
-- Generate the embedding from a natural language prompt
WITH embedding AS (
  SELECT embeddings.values
  FROM ML.PREDICT(
    MODEL DiamondAppraise,
      (SELECT "What is the most valuable diamond?" as prompt)
  )
)
-- Use embedding to find the most similar entries in the database
SELECT id, color, value,
  (APPROX_COSINE_DISTANCE(valueEmbedding,
  embedding.values,
  options => JSON '{"num_leaves_to_search": 10}')) as distance
FROM products @{force_index=valueEmbeddingIndex}, embedding
WHERE valueEmbedding IS NOT NULL
ORDER BY distance
LIMIT 5;
```

You can use `  ML.PREDICT  ` in any DQL/DML statements, such as `  INSERT  ` or `  UPDATE  ` . For example:

``` text
INSERT INTO AppraisedDiamond (id, color, carat, value)
SELECT
  1 AS id,
  color,
  carat,
  value
FROM
  ML.PREDICT(MODEL DiamondAppraise,
  (
    SELECT
      @carat AS carat,
      @cut AS cut,
      @color AS color
  ));
```
