# VoidDB Go SDK

Official Go companion client for VoidDB.

It provides a small, direct API for:

- login and auth
- database and collection access
- document CRUD
- fluent queries
- `.schema` pull and Go model generation via `vdbgo`
- cache access
- schema metadata reads and writes
- Blob field uploads into the built-in S3-compatible storage

## Install

```bash
go get github.com/Nopass0/void_go
```

## Quick start

```go
package main

import (
	"context"
	"log"

	voidorm "github.com/Nopass0/void_go"
)

func main() {
	ctx := context.Background()

	client, err := voidorm.New(voidorm.Config{
		URL: "http://localhost:7700",
	})
	if err != nil {
		log.Fatal(err)
	}

	if _, err := client.Login(ctx, "admin", "admin"); err != nil {
		log.Fatal(err)
	}

	users := client.DB("app").Collection("users")

	id, err := users.Insert(ctx, voidorm.Doc{
		"name":   "Alice",
		"active": true,
	})
	if err != nil {
		log.Fatal(err)
	}

	rows, err := users.Query(ctx,
		voidorm.NewQuery().
			Where("active", voidorm.Eq, true).
			OrderBy("name", voidorm.Asc).
			Limit(25),
	)
	if err != nil {
		log.Fatal(err)
	}

log.Println("inserted", id, "rows", len(rows))
}
```

## Schema and Go type generation

Install the CLI:

```bash
go install github.com/Nopass0/void_go/cmd/vdbgo@latest
```

Then use the short commands:

```bash
vdbgo init
vdbgo pull
vdbgo gen
```

What they do:

- `vdbgo init` creates `.voiddb-go/config.json`, a starter `.schema`, and generated models
- `vdbgo pull` fetches the live schema from the server and regenerates Go types
- `vdbgo gen` regenerates Go types from the local `.schema` file

Default layout:

```text
.voiddb-go/
  config.json
  schema/
    app.schema
  generated/
    models.go
```

## Environment-first setup

```env
VOIDDB_URL=https://db.lowkey.su
VOIDDB_USERNAME=admin
VOIDDB_PASSWORD=your-password
```

```go
client, err := voidorm.NewFromEnv()
if err != nil {
	log.Fatal(err)
}

_, err = client.LoginFromEnv(context.Background())
if err != nil {
	log.Fatal(err)
}
```

Token-based auth also works with:

```env
VOIDDB_URL=https://db.lowkey.su
VOIDDB_TOKEN=your-jwt-token
```

The CLI reads `.env`, `.env.local`, `.voiddb-go/.env`, and `.voiddb-go/.env.local` automatically.

## Queries

The query builder matches the current server query DSL and can also emit raw JSON:

```go
q := voidorm.NewQuery().
	Where("age", voidorm.Gte, 18).
	Where("active", voidorm.Eq, true).
	OrderBy("created_at", voidorm.Desc).
	Limit(10)

payload, _ := q.JSON()
_ = payload

rows, err := client.DB("app").Collection("users").Find(context.Background(), q)
```

## Blob fields and file uploads

Use `Blob` fields in your schema when a document should point at an uploaded file.

```go
import "strings"

ref, err := client.DB("media").Collection("assets").UploadFile(
	context.Background(),
	"asset-123",
	"original",
	strings.NewReader("hello voiddb"),
	voidorm.UploadFileOptions{
		Filename:    "hello.txt",
		ContentType: "text/plain",
	},
)
if err != nil {
	log.Fatal(err)
}

log.Println(ref.URL)
```

Delete the file-backed field with:

```go
err = client.DB("media").Collection("assets").
	DeleteFile(context.Background(), "asset-123", "original")
```

## Cache API

```go
err := client.Cache().Set(ctx, "session:alice", map[string]any{"ok": true}, 3600)
if err != nil {
	log.Fatal(err)
}

var session map[string]any
err = client.Cache().GetJSON(ctx, "session:alice", &session)
```

## Schema metadata

```go
schema, err := client.DB("app").Collection("users").GetSchema(ctx)
if err != nil {
	log.Fatal(err)
}

schema.Fields = append(schema.Fields, voidorm.SchemaField{
	Name: "avatar",
	Type: voidorm.FieldBlob,
})

_, err = client.DB("app").Collection("users").SetSchema(ctx, *schema)
```

The pulled `.schema` file uses the same database-grouped format as the TypeScript ORM:

```prisma
datasource db {
  provider = "voiddb"
  url      = env("VOIDDB_URL")
}

generator client {
  provider = "voiddb-client-go"
  output   = "../generated"
}

database {
  name = "app"

  model User {
    id String @id @map("_id")
    email String @unique
    avatar Blob?
    createdAt DateTime @default(now())
    @@map("users")
  }
}
```

## Links

- Core server repo: https://github.com/Nopass0/void
- Core docs: https://nopass0.github.io/void/
- TypeScript ORM: https://github.com/Nopass0/void_ts

## License

MIT
