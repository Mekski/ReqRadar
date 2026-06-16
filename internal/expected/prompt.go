package expected

import "fmt"

func buildPrompt(company string) string {
	return fmt.Sprintf(promptTemplate, company)
}

// The prompt is grounded (the caller passes it to a Google-Search-backed model),
// so the answer must come from search results — never a guess. The strict
// first-line format makes the response cheap to parse; "unknown" is an explicit,
// expected outcome we honor (leave the estimate blank) rather than fight.
const promptTemplate = `Using Google Search, determine the month when %s typically OPENS
(begins accepting) applications for its SUMMER SOFTWARE ENGINEERING INTERNSHIPS in
the United States, for the upcoming recruiting cycle.

Rules:
- Base your answer ONLY on what you find in current search results. Do not guess.
- Many tech companies open in summer/fall (roughly Jul–Oct); some recruit on a
  rolling/continuous basis with no fixed season.
- If sources indicate a rolling or year-round process, answer "rolling".
- If you cannot find reliable information about THIS company, answer "unknown".

Respond in EXACTLY this format, nothing before it:
ANSWER: <one of: Jan Feb Mar Apr May Jun Jul Aug Sep Oct Nov Dec | rolling | unknown>
WHY: <one sentence describing the evidence you found>`
