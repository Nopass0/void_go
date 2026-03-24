// Package voidorm – query.go implements the fluent query builder.
package voidorm

// Query is an immutable query specification built with a fluent API.
// Each method returns a new Query value; the original is not modified.
//
// Example:
//
//	q := voidorm.NewQuery().
//	    Where("age", voidorm.Gte, 18).
//	    Where("active", voidorm.Eq, true).
//	    OrderBy("name", voidorm.Asc).
//	    Limit(25).
//	    Skip(0)
type Query struct {
	filters []filterClause
	sorts   []sortClause
	limit   *int
	skip    *int
}

// NewQuery returns an empty Query that matches all documents.
func NewQuery() *Query { return &Query{} }

// Where adds a WHERE filter clause.
// The returned Query is a new value; the receiver is unchanged.
func (q *Query) Where(field string, op FilterOp, value interface{}) *Query {
	cp := q.clone()
	cp.filters = append(cp.filters, filterClause{Field: field, Op: op, Value: value})
	return cp
}

// OrderBy adds a sort level.
// Call multiple times to add secondary, tertiary, etc. sort keys.
func (q *Query) OrderBy(field string, dir SortDir) *Query {
	cp := q.clone()
	cp.sorts = append(cp.sorts, sortClause{Field: field, Dir: dir})
	return cp
}

// Limit caps the number of returned documents.
func (q *Query) Limit(n int) *Query {
	cp := q.clone()
	cp.limit = intPtr(n)
	return cp
}

// Skip skips the first n results (for pagination).
func (q *Query) Skip(n int) *Query {
	cp := q.clone()
	cp.skip = intPtr(n)
	return cp
}

// Page is a convenience method that sets Skip and Limit for page-based pagination.
// page is 0-indexed.
func (q *Query) Page(page, pageSize int) *Query {
	return q.Skip(page * pageSize).Limit(pageSize)
}

// toSpec converts the Query into the JSON wire format.
func (q *Query) toSpec() querySpec {
	spec := querySpec{
		Limit: q.limit,
		Skip:  q.skip,
	}
	if len(q.filters) > 0 {
		spec.Where = make([]filterClause, len(q.filters))
		copy(spec.Where, q.filters)
	}
	if len(q.sorts) > 0 {
		spec.OrderBy = make([]sortClause, len(q.sorts))
		copy(spec.OrderBy, q.sorts)
	}
	return spec
}

// clone returns a deep copy of q.
func (q *Query) clone() *Query {
	cp := &Query{
		filters: make([]filterClause, len(q.filters)),
		sorts:   make([]sortClause, len(q.sorts)),
		limit:   q.limit,
		skip:    q.skip,
	}
	copy(cp.filters, q.filters)
	copy(cp.sorts, q.sorts)
	return cp
}

func intPtr(n int) *int { return &n }
