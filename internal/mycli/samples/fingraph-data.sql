-- Sample data for financial graph database

-- Insert persons
INSERT INTO Person (id, name, birthday, country, city) VALUES
  (1, 'Alex', TIMESTAMP '1991-12-21T00:00:00Z', 'Australia', 'Adelaide'),
  (2, 'Dana', TIMESTAMP '1980-10-31T00:00:00Z', 'Czech', 'Moravia'),
  (3, 'Lee', TIMESTAMP '1986-12-07T00:00:00Z', 'China', 'Bejing'),
  (4, 'Lindsey', TIMESTAMP '1987-06-12T00:00:00Z', 'United States', 'Richmond'),
  (5, 'Meredith', TIMESTAMP '1981-07-27T00:00:00Z', 'United States', 'Pasadena');

-- Insert accounts  
INSERT INTO Account (id, create_time, is_blocked, nick_name) VALUES
  (7, TIMESTAMP '2020-01-10T06:22:20.12Z', FALSE, 'Vacation Fund'),
  (16, TIMESTAMP '2020-01-27T17:55:09.12Z', TRUE, 'Vacation Fund'),
  (20, TIMESTAMP '2020-02-18T05:44:20.12Z', FALSE, 'Checking'),
  (21, TIMESTAMP '2022-02-26T03:19:50.12Z', FALSE, 'School Fund');

-- Insert person-account relationships
INSERT INTO PersonOwnAccount (id, account_id, create_time) VALUES
  (1, 7, TIMESTAMP '2020-01-10T06:22:20.12Z'),
  (2, 20, TIMESTAMP '2020-01-27T17:55:09.12Z'),
  (3, 16, TIMESTAMP '2020-02-18T05:44:20.12Z'),
  (1, 16, TIMESTAMP '2020-02-18T05:44:20.12Z'),
  (4, 21, TIMESTAMP '2022-02-26T03:19:50.12Z');

-- Insert account transfers
INSERT INTO AccountTransferAccount (id, to_id, amount, create_time, order_number) VALUES
  (7, 16, 300.0, TIMESTAMP '2020-08-29T15:28:58.12Z', 'A0jV9AUHa'),
  (7, 16, 100.0, TIMESTAMP '2020-10-04T16:55:05.12Z', 'N7j8BHI'),
  (16, 20, 300.0, TIMESTAMP '2020-09-25T02:36:14.12Z', 'I9GyHbD9h'),
  (20, 7, 500.0, TIMESTAMP '2020-10-04T16:55:05.12Z', 'YBbBCH9eh'),
  (20, 16, 200.0, TIMESTAMP '2020-10-17T03:59:40.12Z', 'N781asd');