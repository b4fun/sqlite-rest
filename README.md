<h3 align="center">
<a href="https://gtihub.com/b4fun/sqlite-rest">
<img src="docs/assets/logo.svg" width="180px" height="auto" style="inline-block">
</a>
</h3>

<h4 align="center">
Serve a RESTful API from any SQLite database
</h4>

**sqlite-rest** is similar to [PostgREST][postgrest], but for [SQLite][sqlite]. It's a standalone webs server that adds a RESTful API to any SQLite database.

[PostgREST]: https://postgrest.org/en/stable/
[SQLite]: https://www.sqlite.org/

## Installation

### Build From Source

```
$ go install github.com/b4fun/sqlite-rest@latest
$ sqlite-rest
<omitted help output>
```

### Using docker image

```
$ docker run -it --rm ghcr.io/b4fun/sqlite-rest/server:main
<omitted help output>
```

## Quick Start

Suppose we are serving a book store database with the following schema:

```sql
CREATE TABLE books (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  author TEXT NOT NULL,
  price REAL NOT NULL
);
```

### Create a database

```
$ sqlite3 bookstore.sqlite3 < examples/bookstore/data.sql
```

### Start server

```
$ echo -n "topsecret" > test.token
$ sqlite-rest serve --auth-token-file test.token --security-allow-table books --db-dsn ./bookstore.sqlite3
{"level":"info","ts":1672528510.825417,"logger":"db-server","caller":"sqlite-rest/server.go:121","msg":"server started","addr":":8080"}
... <omitted logs>
```

### Generate authentication token

**NOTE: the following steps create a sample token for testing only, please use a strong password in production.**

- Visit https://jwt.io/
- Choose `HS256` as the algorithm
- Enter `topsecret` as the secret
- Copy the encoded JWT from the encoded output
- Export the token as an environment variable

  ```
  $ export AUTH_TOKEN=<encoded jwt>
  ```


### Querying

**Querying by book id**

```
$ curl -H "Authorization: Bearer $AUTH_TOKEN" http://127.0.0.1:8080/books?id=eq.1
[
 {
  "author": "Stephen King",
  "id": 1,
  "price": 23.54,
  "title": "Fairy Tale"
 }
]
```

**Querying by book price**

```
$ curl -H "Authorization: Bearer $AUTH_TOKEN" http://127.0.0.1:8080/books?price=lt.10
[
 {
  "author": "Alice Hoffman",
  "id": 2,
  "price": 1.99,
  "title": "The Bookstore Sisters: A Short Story"
 },
 {
  "author": "Caroline Peckham",
  "id": 4,
  "price": 8.99,
  "title": "Zodiac Academy 8: Sorrow and Starlight"
 }
]
```

## Features

### Authentication

### Database Migrations

## License

MIT