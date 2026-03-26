// Package voidorm provides a Go client for the VoidDB document store.
package voidorm

// Doc is a raw document map.
type Doc map[string]interface{}

// BlobRef points to an object stored in VoidDB's S3-compatible blob storage.
type BlobRef struct {
	Bucket       string            `json:"_blob_bucket"`
	Key          string            `json:"_blob_key"`
	URL          string            `json:"_blob_url,omitempty"`
	ContentType  string            `json:"content_type,omitempty"`
	ETag         string            `json:"etag,omitempty"`
	Size         int64             `json:"size,omitempty"`
	LastModified string            `json:"last_modified,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// UploadFileOptions controls document field uploads into blob storage.
type UploadFileOptions struct {
	Bucket      string
	Key         string
	Filename    string
	ContentType string
	Metadata    map[string]string
}

// FilterOp is a query comparison operator.
type FilterOp string

const (
	Eq         FilterOp = "eq"
	Ne         FilterOp = "ne"
	Gt         FilterOp = "gt"
	Gte        FilterOp = "gte"
	Lt         FilterOp = "lt"
	Lte        FilterOp = "lte"
	Contains   FilterOp = "contains"
	StartsWith FilterOp = "starts_with"
	In         FilterOp = "in"
)

// SortDir is the sort direction.
type SortDir string

const (
	Asc  SortDir = "asc"
	Desc SortDir = "desc"
)

// QueryNode is a single predicate or logical group in the VoidDB query DSL.
type QueryNode struct {
	AND   []QueryNode `json:"AND,omitempty"`
	OR    []QueryNode `json:"OR,omitempty"`
	Field string      `json:"field,omitempty"`
	Op    FilterOp    `json:"op,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

// QuerySort specifies one level of result ordering.
type QuerySort struct {
	Field string  `json:"field"`
	Dir   SortDir `json:"dir"`
}

// QuerySpec is the JSON body sent to POST /query.
type QuerySpec struct {
	Where   *QueryNode  `json:"where,omitempty"`
	OrderBy []QuerySort `json:"order_by,omitempty"`
	Limit   *int        `json:"limit,omitempty"`
	Skip    *int        `json:"skip,omitempty"`
}

type queryResult struct {
	Results []Doc `json:"results"`
	Count   int64 `json:"count"`
}

// QueryResult holds the fetched documents and total count.
type QueryResult struct {
	Docs  []Doc
	Count int64
}

// TokenPair is the JWT pair returned by the login endpoint.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// User is a VoidDB user account.
type User struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"created_at"`
}

// EngineStats holds engine-level performance metrics.
type EngineStats struct {
	MemtableSize  int64 `json:"memtable_size"`
	MemtableCount int64 `json:"memtable_count"`
	Segments      int64 `json:"segments"`
	CacheLen      int64 `json:"cache_len"`
	CacheUsed     int64 `json:"cache_used"`
	WALSeq        int64 `json:"wal_seq"`
}

// CacheSetRequest is the wire shape used by the cache API.
type CacheSetRequest struct {
	Value string `json:"value"`
	TTL   int    `json:"ttl,omitempty"`
}

// CacheGetResponse is the wire shape returned by the cache API.
type CacheGetResponse struct {
	Value string `json:"value"`
}

// SchemaFieldType is a collection schema field type.
type SchemaFieldType string

const (
	FieldString   SchemaFieldType = "string"
	FieldNumber   SchemaFieldType = "number"
	FieldBoolean  SchemaFieldType = "boolean"
	FieldDateTime SchemaFieldType = "datetime"
	FieldArray    SchemaFieldType = "array"
	FieldObject   SchemaFieldType = "object"
	FieldBlob     SchemaFieldType = "blob"
	FieldRelation SchemaFieldType = "relation"
)

// SchemaRelation stores relation metadata for a field.
type SchemaRelation struct {
	Model      string   `json:"model,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	References []string `json:"references,omitempty"`
	OnDelete   string   `json:"on_delete,omitempty"`
	OnUpdate   string   `json:"on_update,omitempty"`
	Name       string   `json:"name,omitempty"`
}

// SchemaIndex stores index metadata for a collection.
type SchemaIndex struct {
	Name    string   `json:"name,omitempty"`
	Fields  []string `json:"fields"`
	Unique  bool     `json:"unique,omitempty"`
	Primary bool     `json:"primary,omitempty"`
}

// SchemaField describes one field in a collection schema.
type SchemaField struct {
	Name          string          `json:"name"`
	Type          SchemaFieldType `json:"type"`
	Required      bool            `json:"required,omitempty"`
	Default       *string         `json:"default,omitempty"`
	DefaultExpr   *string         `json:"default_expr,omitempty"`
	PrismaType    string          `json:"prisma_type,omitempty"`
	Unique        bool            `json:"unique,omitempty"`
	IsID          bool            `json:"is_id,omitempty"`
	List          bool            `json:"list,omitempty"`
	Virtual       bool            `json:"virtual,omitempty"`
	AutoUpdatedAt bool            `json:"auto_updated_at,omitempty"`
	MappedName    string          `json:"mapped_name,omitempty"`
	Relation      *SchemaRelation `json:"relation,omitempty"`
}

// CollectionSchema is the schema metadata returned by the server.
type CollectionSchema struct {
	Database   string        `json:"database,omitempty"`
	Collection string        `json:"collection,omitempty"`
	Model      string        `json:"model,omitempty"`
	Fields     []SchemaField `json:"fields"`
	Indexes    []SchemaIndex `json:"indexes,omitempty"`
}

// Config holds the client connection configuration.
type Config struct {
	URL     string
	Token   string
	Timeout int
}

// TypegenOptions controls Go model generation from a schema project.
type TypegenOptions struct {
	Package string
}
