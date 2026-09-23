# Summer Crawler

Go implementation of the crawler in `python_version`.

The authorization token is deliberately not embedded in source code. Run it with:

```bash
export SUMMER_AUTHORIZATION='your-token'
go run . -output-dir ./data
```

Use `-friends` to additionally write `friends.json`. Other useful options are
`-base-url` for a test server and `-limit` for the activity page size.
