package main

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/practical-ai-golab2026/foundation/age"
	"github.com/ardanlabs/practical-ai-golab2026/foundation/vector"
	"github.com/jmoiron/sqlx"
)

const (
	perList = 10

	// A feature mentioned by more than 5% of the book, like "type" or "value",
	// connects everything to everything, so the graph hop skips it.
	hubShare = 0.05
)

// list is one ranked answer to "which chunks match?". Kind is where it searched
// and Query which phrasing it searched with: q is the question, r2 the second
// rewrite.
type list struct {
	Kind    string
	Query   string
	Matches []match
}

// Why is what matched inside the chunk: a topic or features. Text and summary
// matches have nothing to add.
type match struct {
	ChunkID int    `db:"chunk_id"`
	Why     string `db:"why"`
}

func recall(ctx context.Context, g *age.Graph, embedder *kronk.Kronk, a analysis, hubs map[string]bool) ([]list, error) {
	phrasings := append([]string{a.Question}, a.Rewrites...)

	queries := make([]string, len(phrasings))
	for i, p := range phrasings {
		queries[i] = asQuery(p)
	}

	vecs, err := embed(ctx, embedder, queries...)
	if err != nil {
		return nil, err
	}

	var lists []list

	for i, v := range vecs {
		label := "q"
		if i > 0 {
			label = fmt.Sprintf("r%d", i)
		}

		for _, table := range []string{"chunk_text", "chunk_summary", "chunk_topic"} {
			matches, err := nearest(ctx, g.DB(), table, v)
			if err != nil {
				return nil, err
			}

			lists = append(lists, list{Kind: strings.TrimPrefix(table, "chunk_"), Query: label, Matches: matches})
		}
	}

	matches, err := featureHop(ctx, g, a.Features, hubs)
	if err != nil {
		return nil, err
	}

	lists = append(lists, list{Kind: "feature", Matches: matches})

	fmt.Println("--- recall ---")
	gray("%d phrasings x 3 tables, plus %d chunks through the feature graph\n", len(phrasings), len(matches))

	return lists, nil
}

// nearest over-fetches because chunk_topic holds several rows per chunk, and
// keeps the best matching topic of each.
func nearest(ctx context.Context, db *sqlx.DB, table string, v []float64) ([]match, error) {
	why := "''"
	if table == "chunk_topic" {
		why = "topic"
	}

	q := fmt.Sprintf(`SELECT chunk_id, %s AS why FROM public.%s ORDER BY embedding <=> $1::vector LIMIT $2`, why, table)

	var rows []match
	if err := db.SelectContext(ctx, &rows, q, vector.FormatPGVector(v), perList*maxTopics); err != nil {
		return nil, fmt.Errorf("nearest %s: %w", table, err)
	}

	var out []match
	for _, m := range rows {
		if !slices.ContainsFunc(out, func(o match) bool { return o.ChunkID == m.ChunkID }) {
			out = append(out, m)
		}
	}

	return out[:min(len(out), perList)], nil
}

// featureHop finds chunks that mention what the question is about, ranked by
// how many matching features they mention. Names match when one holds the other
// as whole words, so "struct padding" meets "padding" but not "add".
func featureHop(ctx context.Context, g *age.Graph, features []string, hubs map[string]bool) ([]match, error) {
	var where []string
	for _, f := range features {
		where = append(where, fmt.Sprintf("f.name CONTAINS %[1]s OR %[1]s CONTAINS f.name", age.Quote(f)))
	}

	if len(where) == 0 {
		return nil, nil
	}

	q := fmt.Sprintf(`
		MATCH (c:Chunk)-[:MENTIONS]->(f:Feature)
		WHERE %s
		RETURN c.id, f.name`, strings.Join(where, " OR "))

	rows, err := g.Query(ctx, q, "id agtype", "name agtype")
	if err != nil {
		return nil, fmt.Errorf("feature hop: %w", err)
	}

	names := map[int][]string{}
	for _, r := range rows {
		name := age.Scalar(r[1])
		if hubs[name] || !slices.ContainsFunc(features, func(f string) bool { return wholeWords(f, name) }) {
			continue
		}

		id, err := age.ParseID(r[0])
		if err != nil {
			return nil, err
		}

		names[id] = append(names[id], name)
	}

	ids := slices.SortedFunc(maps.Keys(names), func(a, b int) int {
		return cmp.Or(cmp.Compare(len(names[b]), len(names[a])), cmp.Compare(a, b))
	})

	var out []match
	for _, id := range ids[:min(len(ids), perList)] {
		out = append(out, match{ChunkID: id, Why: strings.Join(names[id], ", ")})
	}

	return out, nil
}

func wholeWords(a, b string) bool {
	a, b = " "+a+" ", " "+b+" "
	return strings.Contains(a, b) || strings.Contains(b, a)
}

func hubFeatures(ctx context.Context, g *age.Graph) (map[string]bool, error) {
	chunks, err := g.Count(ctx, `MATCH (c:Chunk) RETURN count(c)`)
	if err != nil {
		return nil, fmt.Errorf("count chunks: %w", err)
	}

	if chunks == 0 {
		return nil, fmt.Errorf("nothing ingested yet, run make example16-ingest")
	}

	q := fmt.Sprintf(`
		MATCH (c:Chunk)-[:MENTIONS]->(f:Feature)
		WITH f, count(c) AS n
		WHERE n > %d
		RETURN f.name`, int(float64(chunks)*hubShare))

	rows, err := g.Query(ctx, q, "name agtype")
	if err != nil {
		return nil, fmt.Errorf("hub features: %w", err)
	}

	hubs := map[string]bool{}
	for _, r := range rows {
		hubs[age.Scalar(r[0])] = true
	}

	gray("%d chunks, %d hub features skipped by the graph hop\n", chunks, len(hubs))

	return hubs, nil
}

// vectorOnly is the baseline: the question against chunk_text, top five.
func vectorOnly(ctx context.Context, g *age.Graph, embedder *kronk.Kronk, question string) ([]document, error) {
	vecs, err := embed(ctx, embedder, asQuery(question))
	if err != nil {
		return nil, err
	}

	matches, err := nearest(ctx, g.DB(), "chunk_text", vecs[0])
	if err != nil {
		return nil, err
	}

	var ids []int
	for _, m := range matches[:min(len(matches), 5)] {
		ids = append(ids, m.ChunkID)
	}

	docs, err := loadChunks(ctx, g.DB(), ids)
	if err != nil {
		return nil, err
	}

	fmt.Println("\n--- vector top 5 ---")
	for _, d := range docs {
		gray("#%-4d %-36s %s\n", d.ID, oneLine(d.Section, 36), oneLine(d.Text, 60))
	}

	return docs, nil
}
