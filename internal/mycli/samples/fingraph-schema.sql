-- Financial graph schema from Cloud Spanner Graph documentation
-- Source: https://cloud.google.com/spanner/docs/graph/set-up?hl=en#console

CREATE TABLE Person (
  id               INT64 NOT NULL,
  name             STRING(MAX),
  birthday         TIMESTAMP,
  country          STRING(MAX),
  city             STRING(MAX),
) PRIMARY KEY (id);

CREATE TABLE Account (
  id               INT64 NOT NULL,
  create_time      TIMESTAMP,
  is_blocked       BOOL,
  nick_name        STRING(MAX),
) PRIMARY KEY (id);

CREATE TABLE PersonOwnAccount (
  id               INT64 NOT NULL,
  account_id       INT64 NOT NULL,
  create_time      TIMESTAMP,
  FOREIGN KEY (account_id) REFERENCES Account (id)
) PRIMARY KEY (id, account_id),
  INTERLEAVE IN PARENT Person ON DELETE CASCADE;

CREATE TABLE AccountTransferAccount (
  id               INT64 NOT NULL,
  to_id            INT64 NOT NULL,
  amount           FLOAT64,
  create_time      TIMESTAMP NOT NULL,
  order_number     STRING(MAX),
  FOREIGN KEY (to_id) REFERENCES Account (id)
) PRIMARY KEY (id, to_id, create_time),
  INTERLEAVE IN PARENT Account ON DELETE CASCADE;

-- Create the property graph
CREATE OR REPLACE PROPERTY GRAPH FinGraph
  NODE TABLES (Account, Person)
  EDGE TABLES (
    PersonOwnAccount
      SOURCE KEY (id) REFERENCES Person (id)
      DESTINATION KEY (account_id) REFERENCES Account (id)
      LABEL Owns,
    AccountTransferAccount
      SOURCE KEY (id) REFERENCES Account (id)
      DESTINATION KEY (to_id) REFERENCES Account (id)
      LABEL Transfers
  );