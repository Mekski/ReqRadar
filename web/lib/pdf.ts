import * as pdfjs from "pdfjs-dist";

// Extract resume text in the browser with pdf.js, which reconstructs the spaces
// that LaTeX/Overleaf PDFs encode as kerning (the Go-side extractor dropped these,
// jamming words together). Worker is pinned to the installed version via unpkg.
pdfjs.GlobalWorkerOptions.workerSrc = `https://unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`;

export async function extractPdfText(file: File): Promise<string> {
  const data = await file.arrayBuffer();
  const doc = await pdfjs.getDocument({ data }).promise;
  let out = "";
  for (let i = 1; i <= doc.numPages; i++) {
    const page = await doc.getPage(i);
    const content = await page.getTextContent();
    out += content.items.map((it) => ("str" in it ? it.str : "")).join(" ") + "\n";
  }
  // collapse runs of spaces (keep newlines for structure)
  return out.replace(/[^\S\n]+/g, " ").replace(/\n{3,}/g, "\n\n").trim();
}
