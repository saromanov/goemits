# Contributing to goemits

Thank you for considering a contribution.

## Development

- Go 1.25 or newer is required.
- Unit tests run against an in-process Redis ([miniredis](https://github.com/alicebob/miniredis)) and need no external services:

      go test -race ./...

- Integration tests require a running Redis server:

      GOEMITS_TEST_REDIS=127.0.0.1:6379 go test ./...

- Lint with [golangci-lint](https://golangci-lint.run) v2:

      golangci-lint run

## Pull requests

- Keep each PR focused on a single change.
- Add or update tests for the changed behavior.
- Update the README when the public API changes.
- CI must be green before a PR can be merged.

## Reporting issues

Open a [bug report](https://github.com/saromanov/goemits/issues/new?template=bug_report.md) or a [feature request](https://github.com/saromanov/goemits/issues/new?template=feature_request.md). For open-ended questions use [discussions](https://github.com/saromanov/goemits/discussions).
