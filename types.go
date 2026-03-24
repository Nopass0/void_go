// Package voidorm provides a type-safe Go client for the VoidDB document store.
// It communicates with the VoidDB REST API over HTTP/JSON.
package voidorm

// Doc is an alias for a raw document map.
// Fields may be any JSON-serialisable value.
type Doc map[string]interface{}

// FilterOp is a query comparison operator.
type FilterOp string

const (
	// Eq tests equality.
	Eq FilterOp = "eq"
	// Ne tests inequality.
	Ne FilterOp = "ne"
	// Gt tests greater-than.
	Gt FilterOp = "gt"
	// Gte tests greater-than-or-equal.
	Gte FilterOp = "gte"
	// Lt tests less-than.
	Lt FilterOp = "lt"
	// Lte tests less-than-or-equal.
	Lte FilterOp = "lte"
	// Contains tests substring (string fields) or membership (array fields).
	Contains FilterOp = "contains"
	// StartsWith tests string prefix.
	StartsWith FilterOp = "starts_with"
	// In tests if the field value is one of a list.
	In FilterOp = "in"
)

// SortDir is the sort direction.
type SortDir string

const (
	// Asc sorts ascending.
	Asc SortDir = "asc"
	// Desc sorts descending.
	Desc SortDir = "desc"
)

// filterClause is a single WHERE predicate.
type filterClause struct {
	Field string      `json:"field"`
	Op    FilterOp    `json:"op"`
	Value interface{} `json:"value"`
}

// sortClause specifies one level of result ordering.
type sortClause struct {
	Field string  `json:"field"`
	Dir   SortDir `json:"dir"`
}

// querySpec is the JSON body sent to POST /query.
type querySpec struct {
	Where   []filterClause `json:"where,omitempty"`
	OrderBy []sortClause   `json:"order_by,omitempty"`
	Limit   *int           `json:"limit,omitempty"`
	Skip    *int           `json:"skip,omitempty"`
}

// queryResult is the JSON envelope returned by POST /query.
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

// Config holds the client connection configuration.
type Config struct {
	// URL is the base URL of the VoidDB server, e.g. "http://localhost:7700".
	URL string
	// Token is a pre-issued JWT access token.  If empty, call Client.Login().
	Token string
	// Timeout for HTTP requests (default 30 s).
	Timeout int // seconds
}
