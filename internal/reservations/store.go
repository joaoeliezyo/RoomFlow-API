package reservations

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound         = errors.New("reservation not found")
	ErrConflict         = errors.New("reservation overlaps an active reservation")
	ErrRoomNotFound     = errors.New("room not found")
	ErrAlreadyCancelled = errors.New("reservation already cancelled")
)

type Reservation struct {
	ID          int64      `json:"id"`
	RoomID      int64      `json:"room_id"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      time.Time  `json:"ends_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
}

type scanner interface{ Scan(...any) error }

// DB is implemented by pgxpool.Pool and pgx.Tx.
type DB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct{ db DB }

func NewStore(db DB) *Store { return &Store{db: db} }

func scanReservation(row scanner) (Reservation, error) {
	var reservation Reservation
	var cancelled sql.NullTime
	err := row.Scan(&reservation.ID, &reservation.RoomID, &reservation.StartsAt, &reservation.EndsAt, &cancelled)
	if err != nil {
		return Reservation{}, err
	}
	reservation.StartsAt = reservation.StartsAt.UTC()
	reservation.EndsAt = reservation.EndsAt.UTC()
	if cancelled.Valid {
		at := cancelled.Time.UTC()
		reservation.CancelledAt = &at
	}
	return reservation, nil
}

func (s *Store) Create(ctx context.Context, roomID int64, startsAt, endsAt time.Time) (Reservation, error) {
	reservation, err := scanReservation(s.db.QueryRow(ctx,
		`INSERT INTO reservations (room_id, starts_at, ends_at)
		 VALUES ($1, $2, $3)
		 RETURNING id, room_id, starts_at, ends_at, cancelled_at`,
		roomID, startsAt, endsAt,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch {
			case pgErr.Code == "23P01" && pgErr.ConstraintName == "reservations_no_overlap":
				return Reservation{}, ErrConflict
			case pgErr.Code == "23503" && pgErr.ConstraintName == "reservations_room_id_fkey":
				return Reservation{}, ErrRoomNotFound
			}
		}
	}
	return reservation, err
}

func (s *Store) List(ctx context.Context) ([]Reservation, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, room_id, starts_at, ends_at, cancelled_at
		 FROM reservations ORDER BY starts_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Reservation, 0)
	for rows.Next() {
		reservation, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, reservation)
	}
	return result, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (Reservation, error) {
	reservation, err := scanReservation(s.db.QueryRow(ctx,
		`SELECT id, room_id, starts_at, ends_at, cancelled_at
		 FROM reservations WHERE id = $1`, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrNotFound
	}
	return reservation, err
}

func (s *Store) Cancel(ctx context.Context, id int64) (Reservation, error) {
	reservation, err := scanReservation(s.db.QueryRow(ctx,
		`UPDATE reservations SET cancelled_at = NOW()
		 WHERE id = $1 AND cancelled_at IS NULL
		 RETURNING id, room_id, starts_at, ends_at, cancelled_at`, id,
	))
	if !errors.Is(err, pgx.ErrNoRows) {
		return reservation, err
	}
	_, err = s.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Reservation{}, ErrNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	return Reservation{}, ErrAlreadyCancelled
}
