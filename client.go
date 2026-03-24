// Package voidorm provides a type-safe Go client for VoidDB.
// It communicates with the VoidDB REST API via JSON over HTTP/1.1.
//
// Usage:
//
//	client, err := voidorm.New(voidorm.Config{
//	    URL:   "http://localhost:7700",
//	    Token: os.Getenv("VOID_TOKEN"),
//	})
//	col := client.DB("myapp").Collection("users")
//	id, err := col.Insert(ctx, voidorm.Doc{"name": "Alice", "age": 30})
//	docs, err := col.Find(ctx, voidorm.NewQuery().Where("age", voidorm.Gte, 18))
package voidorm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned when a document does not exist.
var ErrNotFound = fmt.Errorf("voidorm: document not found")

// Client is the VoidDB Go ORM client.
// It is safe for concurrent use by multiple goroutines.
type Client struct {
	mu      sync.RWMutex
	token   string
	refresh string
	cfg     Config
	http    *http.Client
}

// New creates a new Client from cfg.
// Returns an error if the configuration is invalid.
func New(cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("voidorm: URL must not be empty")
	}
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		token: cfg.Token,
		cfg:   cfg,
		http:  &http.Client{Timeout: timeout},
	}, nil
}

// Login authenticates with username/password and stores the resulting tokens.
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

// Me returns the authenticated user.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var u User
	if err := c.get(ctx, "/v1/auth/me", &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Stats returns engine-level performance metrics.
func (c *Client) Stats(ctx context.Context) (*EngineStats, error) {
	var s EngineStats
	if err := c.get(ctx, "/v1/stats", &s); err != nil {
		return nil, err
	}
	return &s, nil
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

// DB returns a Database handle for the given name.
func (c *Client) DB(name string) *Database {
	return &Database{client: c, name: name}
}

// --- Database ----------------------------------------------------------------

// Database provides access to collections within a named VoidDB database.
type Database struct {
	client *Client
	name   string
}

// Collection returns a Collection handle for the given name.
func (db *Database) Collection(name string) *Collection {
	return &Collection{client: db.client, db: db.name, name: name}
}

// ListCollections returns all collection names in the database.
func (db *Database) ListCollections(ctx context.Context) ([]string, error) {
	var res struct {
		Collections []string `json:"collections"`
	}
	if err := db.client.get(ctx, fmt.Sprintf("/v1/databases/%s/collections", db.name), &res); err != nil {
		return nil, err
	}
	return res.Collections, nil
}

// CreateCollection explicitly creates a new collection.
func (db *Database) CreateCollection(ctx context.Context, name string) error {
	return db.client.post(ctx, fmt.Sprintf("/v1/databases/%s/collections", db.name),
		map[string]string{"name": name}, nil)
}

// --- Collection --------------------------------------------------------------

// Collection provides full CRUD and query operations on a VoidDB collection.
type Collection struct {
	client *Client
	db     string
	name   string
}

// path builds the API path for this collection, optionally appending suffix.
func (col *Collection) path(suffix ...string) string {
	p := fmt.Sprintf("/v1/databases/%s/%s", col.db, col.name)
	if len(suffix) > 0 {
		p += "/" + strings.Join(suffix, "/")
	}
	return p
}

// Insert creates a new document and returns its _id.
// If doc has an "_id" key, it is used as the primary key.
func (col *Collection) Insert(ctx context.Context, doc Doc) (string, error) {
	var res struct {
		ID string `json:"_id"`
	}
	if err := col.client.post(ctx, col.path(), doc, &res); err != nil {
		return "", err
	}
	return res.ID, nil
}

// FindByID retrieves a document by its _id.
// Returns ErrNotFound if no document with that id exists.
func (col *Collection) FindByID(ctx context.Context, id string) (Doc, error) {
	var doc Doc
	if err := col.client.get(ctx, col.path(id), &doc); err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return doc, nil
}

// Find returns all documents matching query (nil = all documents).
func (col *Collection) Find(ctx context.Context, q *Query) ([]Doc, error) {
	result, err := col.FindWithCount(ctx, q)
	if err != nil {
		return nil, err
	}
	return result.Docs, nil
}

// FindWithCount returns documents and the total count before limit/skip.
func (col *Collection) FindWithCount(ctx context.Context, q *Query) (*QueryResult, error) {
	spec := querySpec{}
	if q != nil {
		spec = q.toSpec()
	}
	var res queryResult
	if err := col.client.post(ctx, col.path("query"), spec, &res); err != nil {
		return nil, err
	}
	return &QueryResult{Docs: res.Results, Count: res.Count}, nil
}

// Count returns the number of documents in the collection.
func (col *Collection) Count(ctx context.Context) (int64, error) {
	var res struct {
		Count int64 `json:"count"`
	}
	if err := col.client.get(ctx, col.path("count"), &res); err != nil {
		return 0, err
	}
	return res.Count, nil
}

// Replace fully replaces the document with the given id.
func (col *Collection) Replace(ctx context.Context, id string, doc Doc) error {
	return col.client.put(ctx, col.path(id), doc)
}

// Patch partially updates a document (merges the given fields).
// Returns the updated document.
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

// --- HTTP helpers ------------------------------------------------------------

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
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &errBody)
		msg := errBody.Error
		if msg == "" {
			msg = string(data)
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
