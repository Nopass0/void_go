// Package voidorm provides a Go client for the VoidDB REST API.
package voidorm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned when a document or cache entry does not exist.
var ErrNotFound = fmt.Errorf("voidorm: resource not found")

// Client is the root VoidDB client. It is safe for concurrent use.
type Client struct {
	mu      sync.RWMutex
	token   string
	refresh string
	cfg     Config
	http    *http.Client
}

// New creates a new client from the provided config.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("voidorm: URL must not be empty")
	}
	cfg.URL = strings.TrimRight(strings.TrimSpace(cfg.URL), "/")

	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		token: cfg.Token,
		cfg:   cfg,
		http:  &http.Client{Timeout: timeout},
	}, nil
}

// NewFromEnv builds a client from the standard VoidDB environment variables.
func NewFromEnv() (*Client, error) {
	timeout := 0
	if raw := firstNonEmpty(os.Getenv("VOIDDB_TIMEOUT"), os.Getenv("VOID_TIMEOUT")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("voidorm: invalid timeout %q: %w", raw, err)
		}
		timeout = parsed
	}

	return New(Config{
		URL:     firstNonEmpty(os.Getenv("VOIDDB_URL"), os.Getenv("VOID_URL")),
		Token:   firstNonEmpty(os.Getenv("VOIDDB_TOKEN"), os.Getenv("VOID_TOKEN")),
		Timeout: timeout,
	})
}

// Login authenticates with username and password and stores the resulting tokens.
func (c *Client) Login(ctx context.Context, username, password string) (*TokenPair, error) {
	var pair TokenPair
	if err := c.post(ctx, "/v1/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, &pair); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.token = pair.AccessToken
	c.refresh = pair.RefreshToken
	c.mu.Unlock()
	return &pair, nil
}

// LoginFromEnv authenticates using VOIDDB_USERNAME/VOIDDB_PASSWORD.
func (c *Client) LoginFromEnv(ctx context.Context) (*TokenPair, error) {
	username := firstNonEmpty(os.Getenv("VOIDDB_USERNAME"), os.Getenv("VOID_USERNAME"))
	password := firstNonEmpty(os.Getenv("VOIDDB_PASSWORD"), os.Getenv("VOID_PASSWORD"))
	if username == "" || password == "" {
		return nil, fmt.Errorf("voidorm: VOIDDB_USERNAME and VOIDDB_PASSWORD are required")
	}
	return c.Login(ctx, username, password)
}

// Me returns the authenticated user.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var user User
	if err := c.get(ctx, "/v1/auth/me", &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// Stats returns engine-level performance metrics.
func (c *Client) Stats(ctx context.Context) (*EngineStats, error) {
	var stats EngineStats
	if err := c.get(ctx, "/v1/stats", &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// ListDatabases returns all database names.
func (c *Client) ListDatabases(ctx context.Context) ([]string, error) {
	var res struct {
		Databases []string `json:"databases"`
	}
	if err := c.get(ctx, "/v1/databases", &res); err != nil {
		return nil, err
	}
	return res.Databases, nil
}

// CreateDatabase creates a new database.
func (c *Client) CreateDatabase(ctx context.Context, name string) error {
	return c.post(ctx, "/v1/databases", map[string]string{"name": name}, nil)
}

// DropDatabase removes a database.
func (c *Client) DropDatabase(ctx context.Context, name string) error {
	return c.deleteReq(ctx, "/v1/databases/"+pathSegment(name))
}

// DB returns a database handle.
func (c *Client) DB(name string) *Database {
	return &Database{client: c, name: name}
}

// Cache returns a handle for the built-in cache API.
func (c *Client) Cache() *Cache {
	return &Cache{client: c}
}

// Token returns the current bearer token.
func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// Database provides access to collections within a named database.
type Database struct {
	client *Client
	name   string
}

// Collection returns a collection handle for the given name.
func (db *Database) Collection(name string) *Collection {
	return &Collection{client: db.client, db: db.name, name: name}
}

// ListCollections returns all collection names in the database.
func (db *Database) ListCollections(ctx context.Context) ([]string, error) {
	var res struct {
		Collections []string `json:"collections"`
	}
	if err := db.client.get(ctx, fmt.Sprintf("/v1/databases/%s/collections", pathSegment(db.name)), &res); err != nil {
		return nil, err
	}
	return res.Collections, nil
}

// CreateCollection explicitly creates a new collection.
func (db *Database) CreateCollection(ctx context.Context, name string) error {
	return db.client.post(
		ctx,
		fmt.Sprintf("/v1/databases/%s/collections", pathSegment(db.name)),
		map[string]string{"name": name},
		nil,
	)
}

// DropCollection removes a collection.
func (db *Database) DropCollection(ctx context.Context, name string) error {
	return db.client.deleteReq(
		ctx,
		fmt.Sprintf("/v1/databases/%s/collections/%s", pathSegment(db.name), pathSegment(name)),
	)
}

// Collection provides CRUD and query operations for one collection.
type Collection struct {
	client *Client
	db     string
	name   string
}

func (col *Collection) path(suffix ...string) string {
	base := fmt.Sprintf("/v1/databases/%s/%s", pathSegment(col.db), pathSegment(col.name))
	if len(suffix) == 0 {
		return base
	}
	escaped := make([]string, len(suffix))
	for i, part := range suffix {
		escaped[i] = pathSegment(part)
	}
	return base + "/" + strings.Join(escaped, "/")
}

// Insert creates a new document and returns its _id.
func (col *Collection) Insert(ctx context.Context, doc Doc) (string, error) {
	var res struct {
		ID string `json:"_id"`
	}
	if err := col.client.post(ctx, col.path(), doc, &res); err != nil {
		return "", err
	}
	return res.ID, nil
}

// FindByID retrieves a document by _id.
func (col *Collection) FindByID(ctx context.Context, id string) (Doc, error) {
	var doc Doc
	if err := col.client.get(ctx, col.path(id), &doc); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return doc, nil
}

// Get is an alias for FindByID.
func (col *Collection) Get(ctx context.Context, id string) (Doc, error) {
	return col.FindByID(ctx, id)
}

// Find returns documents matching the query. Nil means "all documents".
func (col *Collection) Find(ctx context.Context, q *Query) ([]Doc, error) {
	res, err := col.FindWithCount(ctx, q)
	if err != nil {
		return nil, err
	}
	return res.Docs, nil
}

// Query is an alias for Find.
func (col *Collection) Query(ctx context.Context, q *Query) ([]Doc, error) {
	return col.Find(ctx, q)
}

// FindWithCount returns the documents plus total count before limit/skip.
func (col *Collection) FindWithCount(ctx context.Context, q *Query) (*QueryResult, error) {
	spec := QuerySpec{}
	if q != nil {
		spec = q.Spec()
	}
	var res queryResult
	if err := col.client.post(ctx, col.path("query"), spec, &res); err != nil {
		return nil, err
	}
	return &QueryResult{Docs: res.Results, Count: res.Count}, nil
}

// Count returns the total number of documents in the collection.
func (col *Collection) Count(ctx context.Context) (int64, error) {
	var res struct {
		Count int64 `json:"count"`
	}
	if err := col.client.get(ctx, col.path("count"), &res); err != nil {
		return 0, err
	}
	return res.Count, nil
}

// CountMatching returns the number of matching documents using the query endpoint.
func (col *Collection) CountMatching(ctx context.Context, q *Query) (int64, error) {
	res, err := col.FindWithCount(ctx, q)
	if err != nil {
		return 0, err
	}
	return res.Count, nil
}

// Replace fully replaces the document with the given id.
func (col *Collection) Replace(ctx context.Context, id string, doc Doc) error {
	return col.client.put(ctx, col.path(id), doc)
}

// Patch partially updates a document and returns the updated value.
func (col *Collection) Patch(ctx context.Context, id string, patch Doc) (Doc, error) {
	var updated Doc
	if err := col.client.patchReq(ctx, col.path(id), patch, &updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// Delete removes the document with the given id.
func (col *Collection) Delete(ctx context.Context, id string) error {
	return col.client.deleteReq(ctx, col.path(id))
}

// GetSchema returns the collection schema metadata.
func (col *Collection) GetSchema(ctx context.Context) (*CollectionSchema, error) {
	var schema CollectionSchema
	if err := col.client.get(ctx, col.path("schema"), &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

// SetSchema replaces the collection schema metadata.
func (col *Collection) SetSchema(ctx context.Context, schema CollectionSchema) (*CollectionSchema, error) {
	var updated CollectionSchema
	if err := col.client.doJSON(ctx, http.MethodPut, col.path("schema"), schema, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// UploadFile uploads file contents into a Blob field on the target document.
func (col *Collection) UploadFile(ctx context.Context, id, field string, body io.Reader, opts UploadFileOptions) (*BlobRef, error) {
	if body == nil {
		return nil, fmt.Errorf("voidorm: upload body must not be nil")
	}

	path := col.path(id, "files", field)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, col.client.cfg.URL+path, body)
	if err != nil {
		return nil, err
	}

	query := req.URL.Query()
	if opts.Bucket != "" {
		query.Set("bucket", opts.Bucket)
	}
	if opts.Key != "" {
		query.Set("key", opts.Key)
	}
	if opts.Filename != "" {
		query.Set("filename", opts.Filename)
		req.Header.Set("X-File-Name", opts.Filename)
	}
	req.URL.RawQuery = query.Encode()

	if opts.ContentType != "" {
		req.Header.Set("Content-Type", opts.ContentType)
	} else {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	for key, value := range opts.Metadata {
		req.Header.Set("X-Blob-Meta-"+key, value)
	}

	var res struct {
		Field string  `json:"field"`
		Blob  BlobRef `json:"blob"`
	}
	if err := col.client.do(req, &res); err != nil {
		return nil, err
	}
	return &res.Blob, nil
}

// DeleteFile removes the Blob field and deletes the stored object.
func (col *Collection) DeleteFile(ctx context.Context, id, field string) error {
	return col.client.deleteReq(ctx, col.path(id, "files", field))
}

// BlobURL returns the best URL for the blob reference.
func (col *Collection) BlobURL(ref BlobRef) string {
	if ref.URL != "" {
		return ref.URL
	}
	base := strings.TrimRight(col.client.cfg.URL, "/")
	parts := strings.Split(ref.Key, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return base + "/s3/" + url.PathEscape(ref.Bucket) + "/" + strings.Join(parts, "/")
}

// Cache provides access to VoidDB's key-value cache API.
type Cache struct {
	client *Client
}

// GetRaw fetches a cache entry as a raw string.
func (c *Cache) GetRaw(ctx context.Context, key string) (string, error) {
	var res CacheGetResponse
	if err := c.client.get(ctx, "/v1/cache/"+pathSegment(key), &res); err != nil {
		if isNotFound(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	return res.Value, nil
}

// GetJSON fetches a cache entry and unmarshals it into out.
func (c *Cache) GetJSON(ctx context.Context, key string, out interface{}) error {
	raw, err := c.GetRaw(ctx, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("voidorm: unmarshal cached value: %w", err)
	}
	return nil
}

// Set stores a cache entry. Non-string values are JSON-encoded automatically.
func (c *Cache) Set(ctx context.Context, key string, value interface{}, ttlSeconds int) error {
	var raw string
	switch typed := value.(type) {
	case string:
		raw = typed
	case []byte:
		raw = string(typed)
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("voidorm: marshal cache value: %w", err)
		}
		raw = string(data)
	}

	req := CacheSetRequest{Value: raw}
	if ttlSeconds > 0 {
		req.TTL = ttlSeconds
	}
	return c.client.post(ctx, "/v1/cache/"+pathSegment(key), req, nil)
}

// Delete removes a cache entry.
func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.client.deleteReq(ctx, "/v1/cache/"+pathSegment(key))
}

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body, out interface{}) error {
	return c.doJSON(ctx, http.MethodPost, path, body, out)
}

func (c *Client) put(ctx context.Context, path string, body interface{}) error {
	return c.doJSON(ctx, http.MethodPut, path, body, nil)
}

func (c *Client) patchReq(ctx context.Context, path string, body, out interface{}) error {
	return c.doJSON(ctx, http.MethodPatch, path, body, out)
}

func (c *Client) deleteReq(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.cfg.URL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body, out interface{}) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("voidorm: marshal request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.URL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out interface{}) error {
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("voidorm: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		var body struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &body)
		msg := body.Error
		if msg == "" {
			msg = strings.TrimSpace(string(data))
		}
		return fmt.Errorf("voidorm: %d %s", resp.StatusCode, msg)
	}

	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("voidorm: unmarshal response: %w", err)
		}
	}
	return nil
}

func pathSegment(value string) string {
	return url.PathEscape(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isNotFound(err error) bool {
	return strings.Contains(err.Error(), "404")
}
