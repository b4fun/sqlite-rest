CREATE TABLE books (
  id SERIAL PRIMARY KEY,
  title VARCHAR(255) NOT NULL,
  author VARCHAR(255) NOT NULL,
  price NUMERIC(5,2) NOT NULL
);

INSERT INTO books (id, title, author, price) VALUES
  (1, "Fairy Tale", "Stephen King", 23.54),
  (2, "The Bookstore Sisters: A Short Story", "Alice Hoffman", 1.99),
  (3, "The Invisible Life of Addie LaRue", "V.E. Schwab", 14.99),
  (4, "Zodiac Academy 8: Sorrow and Starlight", "Caroline Peckham", 8.99),
  (5, "He Who Fights with Monsters 8: A LitRPG Adventure", "Shirtaloon, Travis Deverell", 51.99);