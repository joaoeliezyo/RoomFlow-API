package rooms

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrNotFound = errors.New("room not found")
var ErrHasReservations = errors.New("room has reservations")

type Room struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
}

type Update struct {
	Name     *string
	Capacity *int
}

// DB is implemented by both pgxpool.Pool and pgx.Tx.
type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct{ db DB }

func NewStore(db DB) *Store { return &Store{db: db} }

func (s *Store) Create(ctx context.Context, name string, capacity int) (Room, error) {
	room := Room{Name: name, Capacity: capacity}
	err := s.db.QueryRow(ctx,
		`INSERT INTO rooms (name, capacity) VALUES ($1, $2) RETURNING id`,
		name, capacity,
	).Scan(&room.ID)
	return room, err
}

func (s *Store) List(ctx context.Context) ([]Room, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, capacity FROM rooms ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Room, 0)
	for rows.Next() {
		var room Room
		if err := rows.Scan(&room.ID, &room.Name, &room.Capacity); err != nil {
			return nil, err
		}
		result = append(result, room)
	}
	return result, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (Room, error) {
	var room Room
	err := s.db.QueryRow(ctx,
		`SELECT id, name, capacity FROM rooms WHERE id = $1`, id,
	).Scan(&room.ID, &room.Name, &room.Capacity)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, ErrNotFound
	}
	return room, err
}

func (s *Store) Update(ctx context.Context, id int64, update Update) (Room, error) {
	var room Room
	err := s.db.QueryRow(ctx,
		`UPDATE rooms SET name = COALESCE($2, name), capacity = COALESCE($3, capacity)
		 WHERE id = $1 RETURNING id, name, capacity`,
		id, update.Name, update.Capacity,
	).Scan(&room.ID, &room.Name, &room.Capacity)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, ErrNotFound
	}
	return room, err
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM rooms WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "reservations_room_id_fkey" {
			return ErrHasReservations
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
