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

## Usage

### Migration

```
$ sqlite-rest migrate ./migrates
```

### Serve

```
$ echo test > test.token
$ sqlite-rest serve --auth-token-file test.token --security-allow-table test
```

## License

MIT