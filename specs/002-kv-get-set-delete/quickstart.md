# Quickstart: KV Get/Set/Delete with conformance-gated ETag

Verify the feature locally end-to-end: build, run CRUD against a real YDB, and gate `FeatureETag` on the
conformance suite. Assumes the `001-project-scaffold` toolchain (`Makefile`, `deploy/docker-compose.yml`).

## Prerequisites

- Go 1.26.4+ (toolchain auto-downloads if needed)
- Docker (for the local YDB container)
- Repo checked out on branch `002-kv-get-set-delete`

## 1. Start a local YDB

```sh
docker compose -f deploy/docker-compose.yml up -d   # YDB at grpc://localhost:2136/local
```

## 2. Build + unit tests (fast loop)

```sh
make build          # compiles cmd/daprd-ydb and internal/ydbstate
make test           # unit tests: etag parse/branch, expiry filter, request mapping
```

Expected: green. Tests cover `marshalValue` (pure) plus YDB-backed behavior — `ETagMismatch` for
stale/malformed tokens, etag rotation, idempotent delete, and concurrent CAS. The YDB-backed tests skip
automatically when no YDB is reachable, so this step is green with or without the container.

## 3. CRUD smoke (P1) — conformance, basic scenarios

```sh
make conformance    # brings up YDB if needed, runs the conformance suite under -tags conformance
```

At the **P1** stage the harness has its `t.Skip` removed and `operations` set to the basic CRUD
scenarios; `Features()` still returns `[]`. Expected: basic CRUD scenarios pass (SC-002).

## 4. ETag gate (P2)

1. Implement the ETag CAS path (serializable `DoTx`) and add the eTag scenario to `operations`.
2. Run conformance:
   ```sh
   make conformance
   ```
3. **Only when the eTag scenarios pass** (SC-003), flip the capability:
   ```go
   func (s *YDBStore) Features() []state.Feature {
       return []state.Feature{state.FeatureETag}
   }
   ```
4. Re-run `make conformance` — full suite (CRUD + eTag) green. The capability flip and the green suite
   land in the **same commit** (constitution Principle II).

## 5. Verify the gate behaves (SC-007)

```sh
# Before the flip: Features() must NOT list ETag
# After the flip:  Features() must list exactly ETag
go test ./internal/ydbstate -run TestFeatures -v
```

## Manual round-trip (optional)

Bind a tiny app or use `daprd` with `components/ydb.yaml` and exercise save → get → delete; confirm the
value returned by get is byte-identical to what was saved, deleting a missing key is a no-op success, and
a save carrying a stale etag is rejected.

## Done criteria

- `make test` and `make conformance` green (CRUD + eTag).
- `Features()` returns `[]state.Feature{state.FeatureETag}` and nothing else.
- `README.md` updated to list ETag among advertised features and document CRUD usage.
