package models

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type OperationItemStatus string

const (
	OperationItemStatusQueued     OperationItemStatus = "queued"
	OperationItemStatusProcessing OperationItemStatus = "processing"
	OperationItemStatusUpdated    OperationItemStatus = "updated"
	OperationItemStatusSkipped    OperationItemStatus = "skipped"
	OperationItemStatusFailed     OperationItemStatus = "failed"
)

type OperationItem struct {
	ID          string              `json:"id"`
	OperationId string              `json:"operationId"`
	Title       string              `json:"title"` // normalized page title
	Status      OperationItemStatus `json:"status"`
	Error       string              `json:"error"`
	Position    int                 `json:"position"` // preserves input order
	CreatedAt   int64               `json:"createdAt"`
}

func NewOperationItem(operationId, title string, position int) (*OperationItem, error) {
	item := OperationItem{
		ID:          uuid.New().String(),
		OperationId: operationId,
		Title:       title,
		Status:      OperationItemStatusQueued,
		Error:       "",
		Position:    position,
		CreatedAt:   time.Now().Unix(),
	}
	stmt, err := db.Prepare("INSERT INTO operation_item (id, title, status, error, position, operation_id, created_at) VALUES (?,?,?,?,?,?,?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(item.ID, item.Title, item.Status, item.Error, item.Position, item.OperationId, item.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &item, nil
}

func (item *OperationItem) Update() error {
	stmt, err := db.Prepare("UPDATE operation_item SET status=?, error=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(item.Status, item.Error, item.ID)
	if err != nil {
		return err
	}

	return nil
}

func FindOperationItemsByOperationId(operationId string) ([]OperationItem, error) {
	items := make([]OperationItem, 0)

	rows, err := db.Query("SELECT id, title, status, error, position, operation_id, created_at FROM operation_item WHERE operation_id=? ORDER BY position ASC", operationId)
	if err != nil {
		fmt.Println("Error scaning operation items for operation_id ", operationId, err)
		return items, fmt.Errorf("Cannot find requested records")
	}
	defer rows.Close()

	for rows.Next() {
		var item OperationItem
		err := rows.Scan(&item.ID, &item.Title, &item.Status, &item.Error, &item.Position, &item.OperationId, &item.CreatedAt)
		if err != nil {
			fmt.Println("Error parsing operation item", err)
		} else {
			items = append(items, item)
		}
	}

	return items, nil
}

func RequeueFailedOperationItems(operationId string) error {
	stmt, err := db.Prepare("UPDATE operation_item SET status=?, error='' WHERE operation_id=? AND status IN (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(OperationItemStatusQueued, operationId, OperationItemStatusFailed, OperationItemStatusProcessing)
	if err != nil {
		return err
	}

	return nil
}

func initOperationItemTable() {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS operation_item (
		id VARCHAR(255) PRIMARY KEY,
		title TEXT NOT NULL,
		status VARCHAR(50) NOT NULL,
		error TEXT,
		position INT NOT NULL DEFAULT 0,
		operation_id TEXT NOT NULL,
		created_at BIGINT,
		FOREIGN KEY (operation_id) REFERENCES operation(id)
	);`)
	if err != nil {
		log.Fatal(err)
	}
}
