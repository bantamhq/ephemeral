package server

// paginateSlice handles common pagination logic for list endpoints.
// It takes items fetched with limit+1, and returns the paginated items,
// next cursor (if any), and whether there are more items.
func paginateSlice[T any](items []T, limit int, getCursor func(T) string) ([]T, *string, bool) {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	var nextCursor *string
	if hasMore && len(items) > 0 {
		c := getCursor(items[len(items)-1])
		nextCursor = &c
	}

	return items, nextCursor, hasMore
}
