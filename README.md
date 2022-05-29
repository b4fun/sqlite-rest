# sqlite-rest

This is an EXPERIMENT project.

## Usgae

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