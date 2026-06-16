package ats

import "fmt"

func buildPrompt(company string) string {
	return fmt.Sprintf(promptTemplate, company)
}

// Grounded prompt: the model must read real job-board URLs from search results,
// not guess a slug from the company name (the slug is verified against the live
// API afterward regardless, so a wrong guess is caught — but asking for evidence
// keeps the answer honest and reduces wasted verify calls).
const promptTemplate = `Using Google Search, determine whether %s hosts its public software-engineering
job board on Greenhouse or Ashby, and the exact board slug.

How to recognize each (the slug is the path segment after the domain):
- Greenhouse: boards.greenhouse.io/<slug> or job-boards.greenhouse.io/<slug>
  (API: boards-api.greenhouse.io/v1/boards/<slug>/jobs)
- Ashby: jobs.ashbyhq.com/<slug>
  (API: api.ashbyhq.com/posting-api/job-board/<slug>)

Rules:
- Read the slug from a REAL job-board URL you find in search results. Do not invent it.
- If the company uses neither (e.g. Workday, Lever, iCIMS, or a custom site), or you
  cannot find a real board URL, answer PLATFORM: none.

Respond in EXACTLY this format, nothing else:
PLATFORM: <greenhouse|ashby|none>
SLUG: <the slug, or - if none>`
