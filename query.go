// Package voidorm implements the fluent query builder for the Go client.
package voidorm

import "encoding/json"

// Query is an immutable query specification built with a fluent API.
type Query struct {
	filters []QueryNode
	sorts   []QuerySort
	limit   *int
	skip    *int
}

// NewQuery returns an empty Query that matches all documents.
func NewQuery() *Query { return &Query{} }

// Where adds a leaf predicate. Multiple Where calls are combined as AND.
func (q *Query) Where(field string, op FilterOp, value interface{}) *Query {
	cp := q.clone()
	cp.filters = append(cp.filters, QueryNode{Field: field, Op: op, Value: value})
	return cp
}

// WhereNode adds a fully-specified predicate or logical subtree.
func (q *Query) WhereNode(node QueryNode) *Query {
	cp := q.clone()
	cp.filters = append(cp.filters, node)
	return cp
}

// OrderBy adds a sort level.
func (q *Query) OrderBy(field string, dir SortDir) *Query {
	cp := q.clone()
	cp.sorts = append(cp.sorts, QuerySort{Field: field, Dir: dir})
	return cp
}

// Limit caps the number of returned documents.
func (q *Query) Limit(n int) *Query {
	cp := q.clone()
	cp.limit = intPtr(n)
	return cp
}

// Skip skips the first n results.
func (q *Query) Skip(n int) *Query {
	cp := q.clone()
	cp.skip = intPtr(n)
	return cp
}

// Page sets Skip and Limit for page-based pagination.
func (q *Query) Page(page, pageSize int) *Query {
	return q.Skip(page * pageSize).Limit(pageSize)
}

// Spec returns the JSON wire representation for the query.
func (q *Query) Spec() QuerySpec {
	spec := QuerySpec{
		Limit: q.limit,
		Skip:  q.skip,
	}
	if len(q.sorts) > 0 {
		spec.OrderBy = append([]QuerySort(nil), q.sorts...)
	}
	if len(q.filters) == 1 {
		node := q.filters[0]
		spec.Where = &node
		return spec
	}
	if len(q.filters) > 1 {
		node := QueryNode{AND: append([]QueryNode(nil), q.filters...)}
		spec.Where = &node
	}
	return spec
}

// JSON marshals the wire representation for logging or inspection.
func (q *Query) JSON() ([]byte, error) {
	return json.Marshal(q.Spec())
}

func (q *Query) clone() *Query {
	cp := &Query{
		filters: append([]QueryNode(nil), q.filters...),
		sorts:   append([]QuerySort(nil), q.sorts...),
		limit:   q.limit,
		skip:    q.skip,
	}
	return cp
}

func intPtr(n int) *int { return &n }
