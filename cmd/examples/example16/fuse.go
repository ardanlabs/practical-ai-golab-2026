package main

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/ardanlabs/practical-ai-golab2026/foundation/age"
	"github.com/jmoiron/sqlx"
)

const (
	// rrfK is the usual constant for reciprocal rank fusion. It keeps one list's
	// first place from outweighing a chunk that several lists agree on.
	rrfK = 60

	poolSize  = 16
	nextSeeds = 4
)

type document struct {
	ID        int
	Chapter   string
	Section   string
	Text      string
	Hits      []hit
	FusedRank int
	Relevance float64
}

// hit records one way a chunk was found: which search, with which phrasing,
// at what rank, and what matched.
type hit struct {
	Kind  string
	Query string
	Rank  int
	Why   string
}

// fuse scores every chunk by 1/(rrfK+rank) summed over every list it appears
// in, keeps the best, and adds the chunks either side of the top few.
func fuse(ctx context.Context, g *age.Graph, lists []list) ([]document, error) {
	scores := map[int]float64{}
	hits := map[int][]hit{}

	for _, l := range lists {
		for i, m := range l.Matches {
			scores[m.ChunkID] += 1 / float64(rrfK+i+1)
			hits[m.ChunkID] = append(hits[m.ChunkID], hit{Kind: l.Kind, Query: l.Query, Rank: i + 1, Why: m.Why})
		}
	}

	ids := slices.SortedFunc(maps.Keys(scores), func(a, b int) int {
		return cmp.Or(cmp.Compare(scores[b], scores[a]), cmp.Compare(a, b))
	})
	ids = ids[:min(len(ids), poolSize)]

	next, err := nextHop(ctx, g, ids[:min(len(ids), nextSeeds)])
	if err != nil {
		return nil, err
	}

	pooled := len(ids)
	for _, id := range slices.Sorted(maps.Keys(next)) {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
			hits[id] = append(hits[id], hit{Kind: "next", Why: fmt.Sprintf("beside #%d", next[id])})
		}
	}

	docs, err := loadChunks(ctx, g.DB(), ids)
	if err != nil {
		return nil, err
	}

	for i := range docs {
		docs[i].FusedRank = i + 1
		docs[i].Hits = hits[docs[i].ID]
	}

	fmt.Println("--- fuse ---")
	gray("%d lists into %d candidates, plus %d through NEXT\n", len(lists), pooled, len(ids)-pooled)

	return docs, nil
}

// nextHop returns the chunks before and after each seed, and which seed
// brought each one in.
func nextHop(ctx context.Context, g *age.Graph, seeds []int) (map[int]int, error) {
	q := fmt.Sprintf(`
		MATCH (c:Chunk)-[:NEXT]-(n:Chunk)
		WHERE c.id IN [%s]
		RETURN n.id, c.id`, age.Ints(seeds))

	rows, err := g.Query(ctx, q, "id agtype", "seed agtype")
	if err != nil {
		return nil, fmt.Errorf("next hop: %w", err)
	}

	out := map[int]int{}
	for _, r := range rows {
		id, err := age.ParseID(r[0])
		if err != nil {
			return nil, err
		}

		seed, err := age.ParseID(r[1])
		if err != nil {
			return nil, err
		}

		if _, ok := out[id]; !ok {
			out[id] = seed
		}
	}

	return out, nil
}

// loadChunks returns the chunks in the order of ids.
func loadChunks(ctx context.Context, db *sqlx.DB, ids []int) ([]document, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	q, args, err := sqlx.In(`SELECT id, chapter, section, text FROM public.chunk WHERE id IN (?)`, ids)
	if err != nil {
		return nil, err
	}

	var docs []document
	if err := db.SelectContext(ctx, &docs, db.Rebind(q), args...); err != nil {
		return nil, fmt.Errorf("load chunks: %w", err)
	}

	slices.SortFunc(docs, func(a, b document) int {
		return cmp.Compare(slices.Index(ids, a.ID), slices.Index(ids, b.ID))
	})

	return docs, nil
}
