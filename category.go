package bunyan

import (
	"fmt"
	"strings"
	"sync"
)

// entries is a subscope of a category, e.g. SELECT
// entry key is the spanID
type entries map[SpanID]*span

func (e entries) String() string {
	var entryStrings []string
	for spanID2, span2 := range e {
		ent := fmt.Sprintf("<%s: %s>", spanID2, span2.String())
		entryStrings = append(entryStrings, ent)
	}

	return fmt.Sprintf("<Entries: [%s]>", strings.Join(entryStrings, ","))
}

// category is a general grouping of entries, e.g. "database"
type category struct {
	// map of category name to entries
	table   map[string]entries
	comment *[]string
	*sync.Mutex
}

func newEmptyCategory() category {
	return category{
		table:   make(map[string]entries),
		comment: &[]string{},
	}
}

// copy() is not thread safe and must be run under lock or in isolation
func (cat category) copy() *category {
	spanCatCopy := &category{
		table: make(map[string]entries),
	}

	// 1. Deep copy the top-level comment slice
	if cat.comment != nil {
		catComment := make([]string, len(*cat.comment))
		copy(catComment, *cat.comment)
		spanCatCopy.comment = &catComment
	}

	for categoryName, spanEnt := range cat.table {
		// make a new entries as this is a map[SpanID]*span and we don't want to share memory
		spanCatCopy.table[categoryName] = make(entries)

		for entryName, entrySpan := range spanEnt {
			entComment := make([]string, len(entrySpan.comment))
			copy(entComment, entrySpan.comment)

			spanCatCopy.table[categoryName][entryName] = &span{
				updateFn:     nil,
				spanID:       entrySpan.spanID,
				parentSpanID: entrySpan.parentSpanID,
				category:     entrySpan.category,
				start:        entrySpan.start,
				end:          entrySpan.end,
				comment:      entComment,
			}
		}
	}
	return spanCatCopy
}
