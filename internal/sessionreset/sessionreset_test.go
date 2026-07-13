package sessionreset

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		in   string
		want Kind
	}{
		// Stop: bare end/clear commands (skip the LLM, reset memory).
		{"stop", Stop},
		{"Stop.", Stop},
		{"  reset  ", Stop},
		{"okay, start over", Stop},
		{"Start Over!", Stop},
		{"forget that", Stop},
		{"never mind", Stop},
		{"nevermind", Stop},
		{"new conversation", Stop},
		{"clear the conversation please", Stop},
		{"that's all", Stop},

		// Switch: explicit change of subject (reset, then answer normally).
		{"new topic", Switch},
		{"new topic: tell me about ergot", Switch},
		{"let's switch topics", Switch},
		{"can we change the subject?", Switch},
		{"different topic — what's the weather", Switch},
		{"let's talk about something else", Switch},

		// None: ordinary turns, including the follow-up that motivated this work
		// and the deliberately tricky "switch to <same topic>" continuation.
		{"give me a more obscure one", None},
		{"let's switch to the ergotism theory", None},
		{"don't reset my progress", None},
		{"why did they stop dancing?", None},
		{"", None},
		{"   ", None},
	}
	for _, c := range cases {
		if got := Classify(c.in); got != c.want {
			t.Errorf("Classify(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
