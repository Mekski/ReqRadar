package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- Resumes (fit-score input) ----

type Resume struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	ContentText string    `json:"-"` // omitted from list responses; loaded only for scoring
	CreatedAt   time.Time `json:"created_at"`
}

// SaveResume stores an uploaded resume's extracted text and returns its row.
func (s *Store) SaveResume(ctx context.Context, userID int64, filename, contentText string) (Resume, error) {
	sum := sha256.Sum256([]byte(contentText))
	var r Resume
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO resumes (user_id, filename, content_text, content_hash)
		 VALUES ($1, $2, $3, $4) RETURNING id, filename, created_at`,
		userID, filename, contentText, hex.EncodeToString(sum[:])).Scan(&r.ID, &r.Filename, &r.CreatedAt)
	return r, err
}

// ListResumes returns a user's uploaded resumes (newest first), without the text.
func (s *Store) ListResumes(ctx context.Context, userID int64) ([]Resume, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, filename, created_at FROM resumes WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Resume
	for rows.Next() {
		var r Resume
		if err := rows.Scan(&r.ID, &r.Filename, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ResumeText loads the extracted text for a resume (scoping to the user).
func (s *Store) ResumeText(ctx context.Context, userID, resumeID int64) (string, error) {
	var text string
	err := s.Pool.QueryRow(ctx,
		`SELECT content_text FROM resumes WHERE id = $1 AND user_id = $2`, resumeID, userID).Scan(&text)
	return text, err
}

// DeleteResume removes a user's resume.
func (s *Store) DeleteResume(ctx context.Context, userID, resumeID int64) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM resumes WHERE id = $1 AND user_id = $2`, resumeID, userID)
	return err
}

// ---- Fit-score cache ----

// GetFitScore returns the cached result for a (jd, resume) hash pair, if present.
func (s *Store) GetFitScore(ctx context.Context, jdHash, resumeHash string) (json.RawMessage, bool, error) {
	var result json.RawMessage
	err := s.Pool.QueryRow(ctx,
		`SELECT result FROM fit_scores WHERE jd_hash = $1 AND resume_hash = $2`, jdHash, resumeHash).Scan(&result)
	if err == pgx.ErrNoRows {
		return nil, false, nil
	}
	return result, err == nil, err
}

// SaveFitScore caches a result. ON CONFLICT keeps the first (idempotent under a
// race), preserving "one model call per unique pair, ever".
func (s *Store) SaveFitScore(ctx context.Context, jdHash, resumeHash string, postingID *int64, model string, result json.RawMessage) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO fit_scores (jd_hash, resume_hash, posting_id, model, result)
		 VALUES ($1, $2, $3, $4, $5) ON CONFLICT (jd_hash, resume_hash) DO NOTHING`,
		jdHash, resumeHash, postingID, model, result)
	return err
}

// ---- Scoreable JDs (watchlist postings that carry JD text — ATS sources only) ----

type FitJD struct {
	PostingID int64  `json:"posting_id"`
	Company   string `json:"company"`
	Title     string `json:"title"`
	Tier      string `json:"tier"`
	Source    string `json:"source"`
}

// ScoreableJDs lists the user's watchlist postings that have stored JD text (only
// Greenhouse/Ashby carry it), with company + tier, so the fit tab can offer them
// grouped S->C. SimplifyJobs/firehose roles have no JD text and are excluded
// (the user pastes those).
func (s *Store) ScoreableJDs(ctx context.Context, userID int64) ([]FitJD, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT p.id, e.canonical_name, p.title, COALESCE(e.metadata->>'priority',''), src.name
		 FROM postings p
		 JOIN entities e ON e.id = p.entity_id
		 JOIN watchlist w ON w.entity_id = p.entity_id AND w.user_id = $1
		 JOIN sources src ON src.id = p.source_id
		 WHERE p.jd_text IS NOT NULL AND p.jd_text <> ''
		 ORDER BY p.last_seen DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FitJD
	for rows.Next() {
		var j FitJD
		if err := rows.Scan(&j.PostingID, &j.Company, &j.Title, &j.Tier, &j.Source); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// PostingJD returns a posting's stored JD text (for scoring a watchlist role).
func (s *Store) PostingJD(ctx context.Context, postingID int64) (string, error) {
	var jd string
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(jd_text, '') FROM postings WHERE id = $1`, postingID).Scan(&jd)
	return jd, err
}
