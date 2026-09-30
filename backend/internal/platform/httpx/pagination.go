package httpx

import (
	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// Page size bounds for every list operation (API.md, pagination).
const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

// Page is a validated page request. Fetch Limit+1 rows to learn whether another page exists.
type Page struct {
	Limit  int
	Before *uuid.UUID
}

// ParsePage validates the limit and cursor query parameters. The cursor is the external ID of
// the last item of the previous page, so it must carry the listed resource's prefix.
func ParsePage(limit *int, cursor *string, prefix string) (Page, error) {
	p := Page{Limit: DefaultPageSize}
	if limit != nil {
		if *limit < 1 || *limit > MaxPageSize {
			return Page{}, InvalidRequest("limit must be between 1 and 100.", nil)
		}
		p.Limit = *limit
	}
	if cursor != nil && *cursor != "" {
		id, err := ids.Parse(prefix, *cursor)
		if err != nil {
			return Page{}, InvalidRequest("cursor is invalid.", err)
		}
		p.Before = &id
	}
	return p, nil
}

// NextCursor trims rows fetched with limit+1 to the page size and returns the cursor for the
// next page, or nil on the last page.
func NextCursor[T any](rows []T, limit int, prefix string, id func(T) uuid.UUID) ([]T, *string) {
	if len(rows) <= limit {
		return rows, nil
	}
	rows = rows[:limit]
	next := ids.Format(prefix, id(rows[len(rows)-1]))
	return rows, &next
}
