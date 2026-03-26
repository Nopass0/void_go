# VoidDB Go SDK Skill

Use this guide when writing or reviewing Go code that talks to a VoidDB server through `github.com/Nopass0/void_go`.

## What this package covers

- env-based client setup
- username/password login and token auth
- database and collection handles
- document CRUD
- query builder and raw query JSON export
- `.schema` pull, plan, push, and Go model generation with `vdbgo`
- migration authoring, deploy, and status with `vdbgo`
- schema metadata reads and writes
- cache access
- Blob field file uploads and deletes

## Install

```bash
go get github.com/Nopass0/void_go
go install github.com/Nopass0/void_go/cmd/vdbgo@latest
```

## Preferred setup

Use environment variables first:

```env
VOIDDB_URL=http://localhost:7700
VOIDDB_USERNAME=admin
VOIDDB_PASSWORD=admin
```

Then:

```go
client, err := voidorm.NewFromEnv()
if err != nil {
	return err
}

_, err = client.LoginFromEnv(ctx)
if err != nil {
	return err
}
```

If the app already has a bearer token, use `VOIDDB_TOKEN` or `Config.Token`.

The CLI auto-loads:

- `.env`
- `.env.local`
- `.voiddb-go/.env`
- `.voiddb-go/.env.local`

## Schema and type generation

Use the short CLI commands:

```bash
vdbgo init
vdbgo pull
vdbgo plan
vdbgo push
vdbgo gen
vdbgo dev --name add_users
vdbgo deploy
vdbgo status
```

Use them this way:

- `vdbgo init` when a Go project has no VoidDB scaffolding yet
- `vdbgo pull` when the server schema is the source of truth
- `vdbgo plan` before applying schema changes to inspect the diff
- `vdbgo push` to apply the current `.schema` file
- `vdbgo gen` after editing the local `.schema` file
- `vdbgo dev --name ...` to create a migration from the current diff
- `vdbgo deploy` to apply unapplied migration directories
- `vdbgo status` to inspect pending vs applied migrations

Generated Go models default to:

```text
.voiddb-go/generated/models.go
```

The schema file defaults to:

```text
.voiddb-go/schema/app.schema
```

Local migrations default to:

```text
.voiddb-go/migrations
```

## Query rules

Prefer `NewQuery()` over hand-writing raw payloads:

```go
q := voidorm.NewQuery().
	Where("active", voidorm.Eq, true).
	OrderBy("created_at", voidorm.Desc).
	Limit(25)
```

When the caller needs the wire payload for logging or debugging:

```go
payload, err := q.JSON()
```

Multiple `Where(...)` clauses are combined as `AND`, matching the server query DSL.

## Blob field rules

Use `UploadFile(...)` for document-owned files instead of manually patching `_blob_bucket` and `_blob_key`.

```go
ref, err := client.DB("media").Collection("assets").UploadFile(
	ctx,
	"asset-123",
	"original",
	reader,
	voidorm.UploadFileOptions{
		Filename:    "photo.jpg",
		ContentType: "image/jpeg",
	},
)
```

Then use:

- `ref.URL` when the server returned `_blob_url`
- `collection.BlobURL(*ref)` as a safe fallback

Delete file-backed fields with `DeleteFile(...)`.

## Safe defaults

- Prefer `Patch` over `Replace` unless a full replacement is intentional.
- Prefer env-driven config over hardcoded credentials.
- Treat schema changes as non-destructive by default. `plan`, `push`, and migrations must not remove databases that are not declared in the schema file.
- If a live server exposes `/skill.md`, fetch it before assuming route or behavior details.
