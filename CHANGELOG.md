# Changelog

All notable changes of `multi-tour-workflow-execute-refund` (Execute-refund saga) are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [3.1.0] - 2026-11-18

Cut 3 follow-up release. The entries below are the commits of pull request 38 merged into `qa` and re-applied to `release/3.1.0` with `git cherry-pick -x`. The frozen head of `qa` is `ecf1ceecb`.

### Changed

#### HU-WFL-003 - Execute a refund across reservations and cash

- The readme describes the implemented saga and the repository has a pull request template ([PR 38](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/38))
- The store tests run against PostgreSQL 18.6 ([PR 38](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/38))

### Fixed

#### HU-WFL-003 - Execute a refund across reservations and cash

- One refund authorization is paid at most once: the saga allows one running or completed saga per authorization of a tenant, with its migration 005 and rollback ([PR 38](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/38))


## [3.0.0] - 2026-11-14

Cut 3 release. Every entry below is a pull request merged into `qa` and re-applied to `release/3.0.0` with `git cherry-pick -x`; the heading of each group is the user story named in the pull request, and the entry is the title of the pull request without its type. The frozen head of `qa` is `76f5ed106a1d85bd177b88ae804142217dd5b9e3`.

### Added

#### HU-WFL-003 - Execute a refund across reservations and cash

- Read the refund from commercial-reservation after an unknown step 3 ([PR 29](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/29))

### Changed

#### HU-WFL-003 - Execute a refund across reservations and cash

- The configuration of the execute-refund saga (part 1) ([PR 11](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/11))
- The saga tables of the execute-refund saga (part 2) ([PR 12](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/12))
- The domain and use case of the execute-refund saga (part 3) ([PR 13](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/13))
- The participant calls of the execute-refund saga (part 4) ([PR 14](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/14))
- The postgres store of the execute-refund saga (part 5) ([PR 15](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/15))
- The entry route of the execute-refund saga (part 6) ([PR 16](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/16))
- The wiring and the contract of the execute-refund saga (part 7) ([PR 17](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/17))
- The healthcheck that follows LISTEN_ADDR ([PR 19](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/19))
- The checks on pull requests to main ([PR 23](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/23))
- Run the test workflow only on pull requests to develop and qa ([PR 33](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/33))

### Fixed

#### HU-WFL-003 - Execute a refund across reservations and cash

- The key replay refusal, logs and resume ([PR 22](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/22))
- Settle an unknown step 3 by reading the refund ([PR 25](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/25))
- Make the idempotency key unique per tenant ([PR 30](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/30))
- Retry only on a network error, 429 and 5xx ([PR 31](https://github.com/code-corhuila/multi-tour-workflow-execute-refund/pull/31))

[Unreleased]: https://github.com/code-corhuila/multi-tour-workflow-execute-refund/compare/v3.0.0...HEAD
[3.0.0]: https://github.com/code-corhuila/multi-tour-workflow-execute-refund/releases/tag/v3.0.0
