// Package sessionreset detects explicit, user-spoken requests to end or switch
// the current conversation topic. It is deliberately conservative: it only fires
// on unambiguous intent so an ordinary follow-up ("give me a more obscure one")
// is never mistaken for a reset.
//
// It mirrors the small phrase-matcher idiom used elsewhere in the codebase
// (see internal/autopatch/classify.go and internal/codesummary/summary.go).
package sessionreset

import "strings"

// Kind classifies a message's topic-control intent.
type Kind int

const (
	// None means the message is a normal turn — keep the conversation open.
	None Kind = iota
	// Switch means the user explicitly moved to a new topic. The caller should
	// start a fresh conversation and then answer the message normally (it often
	// carries the new topic in the same message).
	Switch
	// Stop means the user issued a bare "end/clear this" command with no new
	// content. The caller should start a fresh conversation and may reply with a
	// short acknowledgement instead of invoking the model.
	Stop
)

func (k Kind) String() string {
	switch k {
	case Switch:
		return "switch"
	case Stop:
		return "stop"
	default:
		return "none"
	}
}

// stopPhrases are matched against the whole normalized message (exact match), so
// they only trigger when the message *is* the command — not when the word merely
// appears inside a longer sentence ("don't reset my progress").
var stopPhrases = map[string]bool{
	"stop":                    true,
	"stop it":                 true,
	"reset":                   true,
	"reset conversation":      true,
	"reset the conversation":  true,
	"start over":              true,
	"start fresh":             true,
	"start again":             true,
	"forget it":               true,
	"forget that":             true,
	"forget everything":       true,
	"forget all that":         true,
	"never mind":              true,
	"nevermind":               true,
	"clear":                   true,
	"clear conversation":      true,
	"clear the conversation":  true,
	"new conversation":        true,
	"end conversation":        true,
	"end the conversation":    true,
	"we're done":              true,
	"were done":               true,
	"that's all":              true,
	"thats all":               true,
}

// switchPhrases are matched as substrings of the normalized message. Each is a
// multi-word phrase that carries unambiguous "change of subject" intent and is
// unlikely to appear while continuing a topic. Note the absence of a bare "switch
// to" — "let's switch to the ergotism theory" continues the topic and must NOT
// reset, whereas "let's switch topics" does.
var switchPhrases = []string{
	"new topic",
	"new subject",
	"change the subject",
	"change subject",
	"change the topic",
	"change topic",
	"different topic",
	"different subject",
	"switch topic", // covers "switch topic" and "switch topics"
	"switch subject",
	"talk about something else",
	"something completely different",
}

// Classify returns the topic-control intent of a message.
func Classify(text string) Kind {
	norm := normalize(text)
	if norm == "" {
		return None
	}
	if stopPhrases[norm] {
		return Stop
	}
	for _, p := range switchPhrases {
		if strings.Contains(norm, p) {
			return Switch
		}
	}
	return None
}

// normalize lowercases, trims wrapping quotes/whitespace, strips trailing
// sentence punctuation, drops a single leading discourse marker and a bracketing
// "please", and collapses internal whitespace — so casual phrasings still match
// the canonical command forms.
func normalize(text string) string {
	s := strings.ToLower(strings.TrimSpace(text))
	s = strings.Trim(s, "\"'")
	s = strings.TrimRight(s, " .!?…,")
	for _, lead := range []string{"ok", "okay", "alright", "hey", "so", "um", "well"} {
		if s == lead {
			continue
		}
		if strings.HasPrefix(s, lead+" ") || strings.HasPrefix(s, lead+",") {
			s = strings.TrimSpace(strings.TrimLeft(s[len(lead):], " ,"))
			break
		}
	}
	s = strings.TrimSpace(strings.TrimPrefix(s, "please "))
	s = strings.TrimSpace(strings.TrimSuffix(s, " please"))
	return strings.Join(strings.Fields(s), " ")
}
