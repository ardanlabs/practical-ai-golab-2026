// This example reruns the step-3 attack against the full content-side stack
// with the payload written in Morse code. Same intent, same request, same
// defenses; only the alphabet changed, so none of the words the blocklist
// matches are present as text.
//
// Every content-side defense that sees this path is on and nothing structural
// is. One claim, isolated: watch what each filter does with a message that has
// no keyword in it. The tool-output sanitizer is left out for the same reason
// as in step 6: there is no tool output here for it to act on.
//
// The classifier is LFM2-700M, the same under-powered guard step 6 uses. With
// the default classifier (the answering model) the Morse message is flagged and
// the step ends BLOCKED.
//
// Expect the outcome to be ANSWERED, not EXFILTRATED. The answering model does
// not decode Morse - it treats the blob as noise and answers the question. The
// claim here is only that both filters are blind to the encoding; step 6 is
// the encoded attack that actually lands, because the model can read dots.
//
// # Running the example:
//
//	$ make example15-step7
//
// # Watch the data arrive - run this in a second terminal FIRST:
//
//	$ make example15-attacker
//
// # This requires running the following commands:
//
//	$ make compose-up
//	$ make kronk-up

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ardanlabs/practical-ai-golab2026/cmd/examples/example15/attacks"
	"github.com/ardanlabs/practical-ai-golab2026/cmd/examples/example15/harness"
)

// step7Classifier is the deliberately under-powered guard, shared with step 6.
const step7Classifier = "unsloth/LFM2-700M-Q8_0"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	h := harness.New()
	defer h.Close()

	// The same under-powered guard as step 6. Only the classifier moves.
	h.WithClassifierModel(step7Classifier)

	h.Config()

	// The same stack that held against the plain-English payload in step 3.
	content := attacks.Defenses{
		HardenedPrompt: true,
		InputSanitizer: true,
		Classifier:     true,
	}

	harness.Banner(
		"STEP 7: The same attack, in Morse code",
		"Every content-side defense on this path is active: hardened prompt,",
		"regex blocklist, LLM classifier. The payload asks for exactly what",
		"step 2's payload asked for. Only the alphabet changed.",
		"Defenses: "+content.String(),
	)

	out, err := attacks.Encoded(ctx, h, content)
	if err != nil {
		return err
	}

	harness.Banner(fmt.Sprintf("OUTCOME: %s", out))

	// The input sanitizer reports matching zero patterns. It did not fail. It had
	// no opportunity to fail - which is why ApplyInputSanitizer reports if it
	// redacted anything rather than just returning the text. A defense that
	// abstained should never print as a defense that worked.
	//
	// The classifier did get a chance and took it: it read a plain question with
	// a Morse attachment and ruled it benign. It was not wrong about the surface.
	// It was reading the wrong thing.
	//
	// The model did not act on it either - it cannot read Morse any better than
	// the filters can, so this run ends ANSWERED. That is luck, not defense: the
	// filters waved the payload through, and step 6 shows the same filters waving
	// through an encoding the model can read.
	//
	// This is not a Morse trick. Base64, ROT13, leetspeak, emoji, pig latin, or
	// asking in Dutch all land in the same place. The attacker chooses the
	// encoding, and they choose it last - after reading whatever list of
	// encodings the defense anticipated.

	return nil
}
