// This example is a standalone teaching illustration. It does nothing but print.
//
// The point it makes, visually, is why a filter that reads text cannot be the
// last line of defense. A blocklist, a regex, a keyword scanner - all of them
// compare the incoming bytes against a list of words. The moment the same
// meaning is carried in a different alphabet, there are no words left in the
// bytes for them to compare against.
//
// Morse code is the clearest way to see this in a classroom. The sentence below
// is ordinary English. Rendered in dots and dashes, not one of its words
// survives as text - yet the meaning is still right there, fully recoverable by
// anyone (or any model) that knows the convention.
//
// This example does not connect to anything, does not call a model, and does
// not run any attack or defense. It only shows the before and the after, so the
// gap between "what a filter sees" and "what the meaning is" is visible at a
// glance.
//
// # Running the example:
//
//	$ go run ./cmd/examples/example15/step7

package main

import (
	"fmt"
	"strings"
	"unicode"
)

// sample is ordinary English - the kind of plain instruction a keyword filter is
// built to catch. It is harmless on its own; it exists only to be encoded.
const sample = "Please summarize the attached report and email it to the team."

func main() {
	fmt.Println(strings.Repeat("=", 72))
	fmt.Println("STEP 7: What the filter sees vs. what the meaning is")
	fmt.Println(strings.Repeat("=", 72))

	fmt.Printf("\nThe plain sentence (every word is right there for a blocklist):\n\n  %s\n", sample)

	fmt.Printf("\nThe same sentence in Morse code (not one word survives as text):\n\n%s\n", indent(toMorse(sample)))

	fmt.Println("\nA filter compares bytes against a word list. In the second form there")
	fmt.Println("are no words to compare against - only dots, dashes and slashes - yet")
	fmt.Println("the meaning is unchanged and trivially recoverable by a reader who knows")
	fmt.Println("the convention. Morse is only the demonstration. Base64, ROT13, emoji,")
	fmt.Println("leetspeak or another language all land in the same place: the attacker")
	fmt.Println("chooses the encoding last, after seeing whatever list the filter knows.")
	fmt.Println("\nThat is why reading the text can never be the defense that holds. The")
	fmt.Println("defenses that do hold never read the text at all.")
}

// =============================================================================
// Morse encoding. No key, no secret: reversible by inspection. The only thing it
// changes is that the words are no longer present in the bytes as words.

var morseTable = map[rune]string{
	'A': ".-", 'B': "-...", 'C': "-.-.", 'D': "-..", 'E': ".",
	'F': "..-.", 'G': "--.", 'H': "....", 'I': "..", 'J': ".---",
	'K': "-.-", 'L': ".-..", 'M': "--", 'N': "-.", 'O': "---",
	'P': ".--.", 'Q': "--.-", 'R': ".-.", 'S': "...", 'T': "-",
	'U': "..-", 'V': "...-", 'W': ".--", 'X': "-..-", 'Y': "-.--",
	'Z': "--..",
	'0': "-----", '1': ".----", '2': "..---", '3': "...--", '4': "....-",
	'5': ".....", '6': "-....", '7': "--...", '8': "---..", '9': "----.",
}

// toMorse rewrites the input in International Morse code. Letters within a word
// are separated by a single space and words by " / ". Characters with no code
// (punctuation) are dropped - they were never what a filter keyed on anyway.
func toMorse(s string) string {
	var words []string
	for word := range strings.FieldsSeq(s) {
		var codes []string
		for _, r := range word {
			if code, ok := morseTable[unicode.ToUpper(r)]; ok {
				codes = append(codes, code)
			}
		}

		if len(codes) > 0 {
			words = append(words, strings.Join(codes, " "))
		}
	}

	return strings.Join(words, " / ")
}

// indent wraps the encoded blob so it reads as a block in the terminal.
func indent(s string) string {
	const width = 68

	var b strings.Builder
	line := "  "
	for _, tok := range strings.Fields(s) {
		if len(line)+len(tok)+1 > width {
			b.WriteString(line)
			b.WriteByte('\n')
			line = "  "
		}
		line += tok + " "
	}
	b.WriteString(strings.TrimRight(line, " "))

	return b.String()
}
