package places

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Place struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) List(userID string) ([]Place, error) {
	rows, err := s.db.Query(`SELECT id,name,address,latitude,longitude FROM favorite_places WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list favorite places: %w", err)
	}
	defer rows.Close()
	items := []Place{}
	for rows.Next() {
		var item Place
		if err = rows.Scan(&item.ID, &item.Name, &item.Address, &item.Latitude, &item.Longitude); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Service) Create(userID string, item Place) (*Place, error) {
	item.Name = strings.TrimSpace(item.Name)
	item.Address = strings.TrimSpace(item.Address)
	if item.Name == "" || len([]rune(item.Name)) > 120 || len([]rune(item.Address)) > 300 {
		return nil, errors.New("place name or address is invalid")
	}
	if item.Latitude < -90 || item.Latitude > 90 || item.Longitude < -180 || item.Longitude > 180 {
		return nil, errors.New("invalid map coordinates")
	}
	err := s.db.QueryRow(`INSERT INTO favorite_places(user_id,name,address,latitude,longitude) VALUES($1,$2,$3,$4,$5) RETURNING id`, userID, item.Name, item.Address, item.Latitude, item.Longitude).Scan(&item.ID)
	if err != nil {
		return nil, fmt.Errorf("save favorite place: %w", err)
	}
	return &item, nil
}
func (s *Service) Delete(userID, id string) error {
	result, err := s.db.Exec(`DELETE FROM favorite_places WHERE user_id=$1 AND id=$2`, userID, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}
