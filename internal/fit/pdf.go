package fit

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractText pulls plain text from an uploaded PDF's bytes. Resumes from
// Overleaf/LaTeX are real text PDFs, so extraction is clean; scanned/image PDFs
// have no text layer and yield little (we surface that as an error so the user
// re-uploads a text PDF rather than getting a silent empty resume).
func ExtractText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("read pdf: %w", err)
	}
	var buf bytes.Buffer
	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract text: %w", err)
	}
	if _, err := io.Copy(&buf, reader); err != nil {
		return "", fmt.Errorf("copy text: %w", err)
	}
	text := strings.TrimSpace(buf.String())
	if len(text) < 50 {
		return "", fmt.Errorf("almost no text found — is this a scanned/image PDF? upload a text-based PDF")
	}
	return text, nil
}
