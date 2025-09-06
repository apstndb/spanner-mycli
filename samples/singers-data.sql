-- Sample data for music database

-- Insert singers
INSERT INTO Singers (SingerId, FirstName, LastName, BirthDate) VALUES
  (1, 'Marc', 'Richards', '1970-09-03'),
  (2, 'Catalina', 'Smith', '1990-08-17'),
  (3, 'Alice', 'Trentor', '1991-10-02'),
  (4, 'Lea', 'Martin', '1991-11-09'),
  (5, 'David', 'Lomond', '1977-01-29');

-- Insert albums with MarketingBudget
INSERT INTO Albums (SingerId, AlbumId, AlbumTitle, MarketingBudget) VALUES
  (1, 1, 'Total Junk', 100000),
  (1, 2, 'Go, Go, Go', 150000),
  (2, 1, 'Green', 200000),
  (2, 2, 'Forever Hold Your Peace', 175000),
  (2, 3, 'Terrified', 300000),
  (3, 1, 'Nothing To Do With Me', 50000),
  (4, 1, 'Play', 80000),
  (5, 1, 'Cake', 120000);

-- Insert songs
INSERT INTO Songs (SingerId, AlbumId, TrackId, SongName, Duration, SongGenre) VALUES
  (1, 1, 1, 'Not About The Guitar', 213, 'BLUES'),
  (1, 1, 2, 'The Second Time', 227, 'ROCK'),
  (1, 2, 1, 'Starting Again', 288, 'ROCK'),
  (1, 2, 2, 'Let''s Get Back Together', 194, 'COUNTRY'),
  (1, 2, 3, 'I Knew You Were Magic', 203, 'BLUES'),
  (2, 1, 1, 'Nothing Is The Same', 303, 'METAL'),
  (2, 1, 2, 'Respect', 198, 'ELECTRONIC'),
  (2, 2, 1, 'The Second Title', 254, 'BLUES'),
  (2, 3, 1, 'Fight Story', 320, 'ROCK'),
  (3, 1, 1, 'Let Me', 204, 'COUNTRY'),
  (3, 1, 2, 'I Can''t Go On', 221, 'BLUES'),
  (4, 1, 1, 'Our Time', 197, 'ROCK'),
  (4, 1, 2, 'My Dear One', 196, 'BLUES'),
  (5, 1, 1, 'Blue', 238, 'BLUES'),
  (5, 1, 2, 'Sorry', 201, 'ROCK'),
  (5, 1, 3, 'Another Day', 194, 'COUNTRY');

-- Insert concerts with TicketPrices
INSERT INTO Concerts (VenueId, SingerId, ConcertDate, BeginTime, EndTime, TicketPrices) VALUES
  (1, 1, '2018-05-01', TIMESTAMP '2018-05-01T19:00:00Z', TIMESTAMP '2018-05-01T21:00:00Z', [25, 50, 100]),
  (1, 2, '2018-05-01', TIMESTAMP '2018-05-01T21:30:00Z', TIMESTAMP '2018-05-01T23:30:00Z', [30, 60, 120]),
  (2, 1, '2019-01-15', TIMESTAMP '2019-01-15T20:00:00Z', TIMESTAMP '2019-01-15T22:00:00Z', [35, 70, 140]),
  (3, 2, '2019-06-20', TIMESTAMP '2019-06-20T18:00:00Z', TIMESTAMP '2019-06-20T20:00:00Z', [40, 80, 160]),
  (4, 3, '2020-09-10', TIMESTAMP '2020-09-10T19:30:00Z', TIMESTAMP '2020-09-10T21:30:00Z', [20, 40, 80]),
  (5, 4, '2021-03-25', TIMESTAMP '2021-03-25T20:00:00Z', TIMESTAMP '2021-03-25T22:30:00Z', [25, 45, 90]);