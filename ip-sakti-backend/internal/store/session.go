package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSessionNotFound is returned when a requested session does not exist.
var ErrSessionNotFound = errors.New("session not found")

// Session represents the persisted state of a single user interaction chain.
type Session struct {
	ID                   string
	RawDescription       string
	ClarificationAnswers map[string]string
	Classification       *domain.ClassificationResult
	ClassificationDone   bool
	Roadmap              *domain.IPRoadmap
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Store wraps a pgxpool and provides session CRUD operations.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by the supplied connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateSession persists a new session with the given raw description and
// returns the created Session (ID populated from Postgres gen_random_uuid()).
func (s *Store) CreateSession(ctx context.Context, description string) (*Session, error) {
	const q = `
		INSERT INTO sessions (raw_description)
		VALUES ($1)
		RETURNING id, raw_description, clarification_answers,
		          classification, classification_done, roadmap,
		          created_at, updated_at`

	row := s.pool.QueryRow(ctx, q, description)
	return scanSession(row)
}

// GetSession retrieves a session by its UUID. Returns ErrSessionNotFound if
// no matching row exists.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	const q = `
		SELECT id, raw_description, clarification_answers,
		       classification, classification_done, roadmap,
		       created_at, updated_at
		FROM   sessions
		WHERE  id = $1`

	row := s.pool.QueryRow(ctx, q, id)
	sess, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	return sess, err
}

// UpdateClassification stores the classifier output and marks classification
// as done.
func (s *Store) UpdateClassification(ctx context.Context, id string, result *domain.ClassificationResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal classification: %w", err)
	}
	const q = `
		UPDATE sessions
		SET    classification      = $2,
		       classification_done = TRUE,
		       updated_at          = NOW()
		WHERE  id = $1`

	tag, err := s.pool.Exec(ctx, q, id, data)
	if err != nil {
		return fmt.Errorf("update classification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// UpdateClarificationAnswers merges new answers into the existing JSONB map.
func (s *Store) UpdateClarificationAnswers(ctx context.Context, id string, answers map[string]string) error {
	data, err := json.Marshal(answers)
	if err != nil {
		return fmt.Errorf("marshal answers: %w", err)
	}
	const q = `
		UPDATE sessions
		SET    clarification_answers = clarification_answers || $2::jsonb,
		       updated_at            = NOW()
		WHERE  id = $1`

	tag, err := s.pool.Exec(ctx, q, id, data)
	if err != nil {
		return fmt.Errorf("update clarification answers: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// UpdateRoadmap persists the synthesised IP roadmap onto the session.
func (s *Store) UpdateRoadmap(ctx context.Context, id string, roadmap *domain.IPRoadmap) error {
	data, err := json.Marshal(roadmap)
	if err != nil {
		return fmt.Errorf("marshal roadmap: %w", err)
	}
	const q = `
		UPDATE sessions
		SET    roadmap    = $2,
		       updated_at = NOW()
		WHERE  id = $1`

	tag, err := s.pool.Exec(ctx, q, id, data)
	if err != nil {
		return fmt.Errorf("update roadmap: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// scanSession reads a pgx Row into a Session, unmarshalling JSONB columns.
func scanSession(row pgx.Row) (*Session, error) {
	var (
		sess              Session
		clarifyRaw        []byte
		classificationRaw []byte
		roadmapRaw        []byte
	)

	err := row.Scan(
		&sess.ID,
		&sess.RawDescription,
		&clarifyRaw,
		&classificationRaw,
		&sess.ClassificationDone,
		&roadmapRaw,
		&sess.CreatedAt,
		&sess.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	sess.ClarificationAnswers = make(map[string]string)
	if len(clarifyRaw) > 0 {
		if err := json.Unmarshal(clarifyRaw, &sess.ClarificationAnswers); err != nil {
			return nil, fmt.Errorf("unmarshal clarification_answers: %w", err)
		}
	}

	if len(classificationRaw) > 0 {
		sess.Classification = new(domain.ClassificationResult)
		if err := json.Unmarshal(classificationRaw, sess.Classification); err != nil {
			return nil, fmt.Errorf("unmarshal classification: %w", err)
		}
	}

	if len(roadmapRaw) > 0 {
		sess.Roadmap = new(domain.IPRoadmap)
		if err := json.Unmarshal(roadmapRaw, sess.Roadmap); err != nil {
			return nil, fmt.Errorf("unmarshal roadmap: %w", err)
		}
	}

	return &sess, nil
}
