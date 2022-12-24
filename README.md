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
CREATE TABLE book (
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
$ echo "topsecret" > test.token
$ sqlite-rest serve --auth-token-file test.token --security-allow-table book
```

## Features

### Authentication

### Database Migrations

## License

MIT