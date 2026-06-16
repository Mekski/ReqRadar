package sentiment

import "fmt"

// buildPrompt builds the grounded-search sentiment prompt for a company. It is
// heavily anti-hallucination: the model must answer only from what it finds, and
// say "not enough public information found" rather than invent specifics.
func buildPrompt(company string) string {
	return fmt.Sprintf(promptTemplate, company, company)
}

const promptTemplate = `You are a careful research analyst helping a candidate evaluate %q as a place to do
a SOFTWARE ENGINEERING INTERNSHIP. Using ONLY information you find via web search
(Reddit such as r/csMajors and r/cscareerquestions, Hacker News, blogs, news,
forums), write a structured report on what the developer/student community actually
says.

STRICT ANTI-HALLUCINATION RULES (follow exactly):
- Use ONLY facts supported by your search results. NEVER invent specifics — no made-up
  number of interview rounds, stipend amounts, ratings, or quotes.
- If you cannot find real information for a section or a bullet, write exactly:
  "_Not enough public information found._" — do NOT guess or pad with generic filler.
- Prefer recent discussion (last ~2 years) and intern / new-grad perspectives.
- Report conflicting takes honestly ("some say X, others say Y").

Output GitHub-flavored MARKDOWN with EXACTLY these sections (use ## headers):

## Overall
A one-word verdict on "do people like interning/working here?" in bold — one of
**Positive**, **Mixed**, **Negative**, or **Insufficient data** — then 1–2 sentences.

## Prestige & reputation
How the company is regarded in industry and among students.

## Culture — liked vs disliked
A short **Liked:** bullet list and a **Disliked:** bullet list. Only real, sourced points.

## Interview process
Be specific where the data exists; otherwise say not found per item:
- **Online assessment (OA):** difficulty and format
- **Rounds:** how many and what kind
- **Timeline:** roughly how long from application to offer
- **Overall difficulty:** easy / medium / hard
- Any notable tips or recurring patterns people mention.

## Intern pay & housing
Reported hourly/monthly pay, and **housing stipend / relocation** if mentioned. If you
find no housing-stipend information, say so explicitly — do not guess.

## Return offers & conversion
What people say about intern → return-offer rates and full-time conversion.

## Early-talent programs & ways in
Specific programs or paths toward an internship or direct consideration — e.g.
student / early-talent or "insider"/ambassador programs, diversity & inclusion
pipelines, hackathons or coding challenges the company runs, info sessions / campus
recruiting, and referral routes. For each, give the **program name**, what it is in a
few words, who it's for, and **timing/deadlines** if found. List only real, sourced
programs; if you find none, write "_Not enough public information found._"

## Watch-outs
Common complaints or red flags, if any.

## Confidence
How much public information you found (**High** / **Medium** / **Low**) and any caveats
(e.g. "mostly older threads", "little intern-specific data").

Keep it scannable and honest. Do NOT include a sources/links section — sources are
attached separately. Company: %q`
