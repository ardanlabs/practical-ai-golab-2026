package main

import (
	"os"
	"regexp"
	"strings"
)

const (
	chunkTokens = 400

	// About 15%. A sentence cut by a chunk boundary is repeated whole at the
	// start of the next chunk, so neither half has to stand alone.
	chunkOverlap = 60
)

type chunk struct {
	ID      int
	Chapter string
	Section string
	Text    string
}

var (
	pageRE    = regexp.MustCompile(`^\d+$`)
	chapterRE = regexp.MustCompile(`^Chapter \d+: .+$`)
	sectionRE = regexp.MustCompile(`^\d{1,2}(\.\d{1,2})+ [A-Z].{0,60}$`)
)

// chunkBook never lets a chunk cross a section heading, and only carries
// overlap between chunks of the same section.
func chunkBook(file string) ([]chunk, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	type line struct {
		text   string
		tokens int
	}

	var (
		chunks           []chunk
		cur              []line
		used, fresh      int
		chapter, section = "Front Matter", "Front Matter"
	)

	emit := func(carry int) {
		if fresh > 0 {
			texts := make([]string, len(cur))
			for i, l := range cur {
				texts[i] = l.text
			}

			chunks = append(chunks, chunk{
				ID:      len(chunks),
				Chapter: chapter,
				Section: section,
				Text:    strings.Join(texts, "\n"),
			})
		}

		start, kept := len(cur), 0
		for start > 0 && kept+cur[start-1].tokens <= carry {
			start--
			kept += cur[start].tokens
		}

		cur, used, fresh = cur[start:], kept, 0
	}

	for text := range strings.Lines(string(data)) {
		text = strings.TrimSpace(text)

		switch {
		case text == "", pageRE.MatchString(text), strings.Contains(text, "....."):
			continue

		case chapterRE.MatchString(text):
			emit(0)
			chapter, section = text, text

		case sectionRE.MatchString(text):
			emit(0)
			section = text
		}

		tokens := countTokens(text)
		if used+tokens > chunkTokens {
			emit(chunkOverlap)
		}

		cur = append(cur, line{text, tokens})
		used += tokens
		fresh++
	}

	emit(0)

	return chunks, nil
}
