package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/practical-ai-golab2026/foundation/age"
	"github.com/ardanlabs/practical-ai-golab2026/foundation/vector"
)

// One table per kind of vector, so every match can say where it came from.
// Tables are qualified with public because AGE puts ag_catalog first on the
// search_path.
var schema = []string{
	`CREATE EXTENSION IF NOT EXISTS vector`,
	`DROP TABLE IF EXISTS public.chunk_topic, public.chunk_summary, public.chunk_text, public.chunk`,

	`CREATE TABLE public.chunk (
		id      INT PRIMARY KEY,
		chapter TEXT NOT NULL,
		section TEXT NOT NULL,
		text    TEXT NOT NULL
	)`,

	fmt.Sprintf(`CREATE TABLE public.chunk_text (
		chunk_id  INT PRIMARY KEY REFERENCES public.chunk (id),
		embedding VECTOR(%d) NOT NULL
	)`, dimensions),

	fmt.Sprintf(`CREATE TABLE public.chunk_summary (
		chunk_id  INT PRIMARY KEY REFERENCES public.chunk (id),
		summary   TEXT NOT NULL,
		embedding VECTOR(%d) NOT NULL
	)`, dimensions),

	fmt.Sprintf(`CREATE TABLE public.chunk_topic (
		chunk_id  INT NOT NULL REFERENCES public.chunk (id),
		topic     TEXT NOT NULL,
		embedding VECTOR(%d) NOT NULL
	)`, dimensions),

	`CREATE INDEX ON public.chunk_text USING hnsw (embedding vector_cosine_ops)`,
	`CREATE INDEX ON public.chunk_summary USING hnsw (embedding vector_cosine_ops)`,
	`CREATE INDEX ON public.chunk_topic USING hnsw (embedding vector_cosine_ops)`,
}

func store(ctx context.Context, g *age.Graph, embedder, chat *kronk.Kronk, chunks []chunk) error {
	for _, q := range schema {
		if _, err := g.DB().ExecContext(ctx, q); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}

	if err := g.Exec(ctx, `MATCH (n) DETACH DELETE n`); err != nil {
		return fmt.Errorf("clear graph: %w", err)
	}

	start := time.Now()
	features := map[string]bool{}

	// Chunks are enriched one at a time to keep this readable. Enriching several
	// at once is the place to optimize further.
	for _, c := range chunks {
		fmt.Printf("\r\033[Kingesting chunk %d of %d, %d features so far", c.ID+1, len(chunks), len(features))

		if err := storeChunk(ctx, g, c); err != nil {
			return fmt.Errorf("store chunk %d: %w", c.ID, err)
		}

		e, err := enrich(ctx, chat, c)
		if err != nil {
			return fmt.Errorf("enrich chunk %d: %w", c.ID, err)
		}

		if err := storeEnrichment(ctx, g, embedder, c, e); err != nil {
			return fmt.Errorf("store enrichment %d: %w", c.ID, err)
		}

		for _, f := range e.Features {
			features[f] = true
		}
	}

	fmt.Printf("\ningested %d chunks, %d features in %v\n", len(chunks), len(features), time.Since(start).Round(time.Second))

	return nil
}

func storeChunk(ctx context.Context, g *age.Graph, c chunk) error {
	if _, err := g.DB().ExecContext(ctx, `INSERT INTO public.chunk VALUES ($1, $2, $3, $4)`,
		c.ID, c.Chapter, c.Section, c.Text); err != nil {
		return err
	}

	if err := g.Exec(ctx, fmt.Sprintf(`CREATE (:Chunk {id: %d})`, c.ID)); err != nil {
		return err
	}

	if c.ID == 0 {
		return nil
	}

	return g.Exec(ctx, fmt.Sprintf(`
		MATCH (a:Chunk {id: %d}), (b:Chunk {id: %d})
		CREATE (a)-[:NEXT]->(b)`, c.ID-1, c.ID))
}

// The graph only stores ids and feature names. Text lives in the tables.
func storeEnrichment(ctx context.Context, g *age.Graph, embedder *kronk.Kronk, c chunk, e enrichment) error {
	texts := []string{asDocument(c.Section, c.Text), asDocument(c.Section, e.Summary)}
	for _, t := range e.Topics {
		texts = append(texts, asDocument("none", t))
	}

	vecs, err := embed(ctx, embedder, texts...)
	if err != nil {
		return err
	}

	db := g.DB()

	if _, err := db.ExecContext(ctx, `INSERT INTO public.chunk_text VALUES ($1, $2::vector)`,
		c.ID, vector.FormatPGVector(vecs[0])); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO public.chunk_summary VALUES ($1, $2, $3::vector)`,
		c.ID, e.Summary, vector.FormatPGVector(vecs[1])); err != nil {
		return err
	}

	for i, t := range e.Topics {
		if _, err := db.ExecContext(ctx, `INSERT INTO public.chunk_topic VALUES ($1, $2, $3::vector)`,
			c.ID, t, vector.FormatPGVector(vecs[2+i])); err != nil {
			return err
		}
	}

	for _, f := range e.Features {
		if err := g.Exec(ctx, fmt.Sprintf(`MERGE (:Feature {name: %s})`, age.Quote(f))); err != nil {
			return err
		}

		if err := g.Exec(ctx, fmt.Sprintf(`
			MATCH (c:Chunk {id: %d}), (f:Feature {name: %s})
			CREATE (c)-[:MENTIONS]->(f)`, c.ID, age.Quote(f))); err != nil {
			return err
		}
	}

	return nil
}
