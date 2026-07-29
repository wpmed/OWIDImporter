package models

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type (
	OperationStatus string
	OperationType   string
)

const (
	OperationStatusQueued     OperationStatus = "queued"
	OperationStatusProcessing OperationStatus = "processing"
	OperationStatusDone       OperationStatus = "done"
	OperationStatusFailed     OperationStatus = "failed"
	OperationStatusCancelled  OperationStatus = "cancelled"
)

const (
	OperationTypeUpdateDefaults OperationType = "update_defaults"
)

type Operation struct {
	ID              string          `json:"id"`
	UserId          string          `json:"userId"`
	Type            OperationType   `json:"type"`
	Status          OperationStatus `json:"status"`
	Payload         string          `json:"payload"`  // JSON blob for operation type specific configuration
	Archived        int             `json:"archived"` // 0 for false, 1 for true
	LastOperationAt int64           `json:"lastOperationAt"`
	CreatedAt       int64           `json:"createdAt"`
}

func NewOperation(userId string, operationType OperationType, payload string) (*Operation, error) {
	operation := Operation{
		ID:              uuid.New().String(),
		UserId:          userId,
		Type:            operationType,
		Status:          OperationStatusQueued,
		Payload:         payload,
		Archived:        0,
		LastOperationAt: time.Now().Unix(),
		CreatedAt:       time.Now().Unix(),
	}
	stmt, err := db.Prepare("INSERT INTO operation (id, user_id, type, status, payload, archived, last_operation_at, created_at) VALUES (?,?,?,?,?,?,?,?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		operation.ID,
		operation.UserId,
		operation.Type,
		operation.Status,
		operation.Payload,
		operation.Archived,
		operation.LastOperationAt,
		operation.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &operation, nil
}

func (operation *Operation) Update() error {
	stmt, err := db.Prepare("UPDATE operation SET status=?, payload=?, archived=?, last_operation_at=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(operation.Status, operation.Payload, operation.Archived, operation.LastOperationAt, operation.ID)
	if err != nil {
		return err
	}

	return nil
}

func (operation *Operation) Reload() error {
	err := db.QueryRow(
		"SELECT id, user_id, type, status, payload, archived, last_operation_at, created_at FROM operation WHERE id=?",
		operation.ID,
	).Scan(&operation.ID, &operation.UserId, &operation.Type, &operation.Status, &operation.Payload, &operation.Archived, &operation.LastOperationAt, &operation.CreatedAt)
	if err != nil {
		fmt.Println("Error reloading operation for id: ", operation.ID, err)
		return fmt.Errorf("error reloading operation: %w", err)
	}

	return nil
}

func UpdateOperationLastOperationAt(id string) error {
	stmt, err := db.Prepare("UPDATE operation SET last_operation_at=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(time.Now().Unix(), id)
	if err != nil {
		return err
	}
	return nil
}

func FindOperationById(id string) (*Operation, error) {
	var operation Operation
	err := db.QueryRow("SELECT id, user_id, type, status, payload, archived, last_operation_at, created_at FROM operation WHERE id=?", id).
		Scan(
			&operation.ID,
			&operation.UserId,
			&operation.Type,
			&operation.Status,
			&operation.Payload,
			&operation.Archived,
			&operation.LastOperationAt,
			&operation.CreatedAt,
		)
	if err != nil {
		println("Error scaning for id ", id, err)
		return nil, fmt.Errorf("Cannot find requested record")
	}

	return &operation, nil
}

func FindOperationsByUserId(userId string, archived, skip, limit int) (*[]Operation, int, error) {
	operations := make([]Operation, 0)
	condition := "user_id=? AND archived=?"
	args := []interface{}{userId, archived}

	queryArgs := append(args, limit, skip)
	rows, err := db.Query(fmt.Sprintf("SELECT id, user_id, type, status, payload, archived, last_operation_at, created_at FROM operation WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?", condition), queryArgs...)
	if err != nil {
		fmt.Println("Error scaning for user_id ", userId, err)
		return nil, 0, fmt.Errorf("Cannot find requested record")
	}
	defer rows.Close()

	for rows.Next() {
		var operation Operation
		rows.Scan(
			&operation.ID,
			&operation.UserId,
			&operation.Type,
			&operation.Status,
			&operation.Payload,
			&operation.Archived,
			&operation.LastOperationAt,
			&operation.CreatedAt,
		)
		operations = append(operations, operation)
	}

	row := db.QueryRow(fmt.Sprintf("SELECT COUNT(id) as c FROM operation WHERE %s", condition), args...)
	count := 0
	row.Scan(&count)
	return &operations, count, nil
}

func FindStalledOperations() (*[]Operation, error) {
	operations := make([]Operation, 0)
	timeThreshold := time.Now().Unix() - 60*5 // 5 Min threshold
	rows, err := db.Query("SELECT id, user_id, type, status, payload, archived, last_operation_at, created_at FROM operation WHERE status=? AND last_operation_at <= ?", OperationStatusProcessing, timeThreshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var operation Operation
		rows.Scan(&operation.ID, &operation.UserId, &operation.Type, &operation.Status, &operation.Payload, &operation.Archived, &operation.LastOperationAt, &operation.CreatedAt)
		operations = append(operations, operation)
	}

	return &operations, nil
}

func FindProcessingOperationsCount() (int, error) {
	rows, err := db.Query("SELECT COUNT(id) FROM operation WHERE status=?", OperationStatusProcessing)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int
	if rows.Next() {
		err = rows.Scan(&count)
		if err != nil {
			return 0, err
		}
	}

	return count, nil
}

func FindNextOperationToProcess() (*Operation, error) {
	rows, err := db.Query("SELECT id, user_id, type, status, payload, archived, last_operation_at, created_at FROM operation WHERE status=? ORDER BY created_at ASC LIMIT 1", OperationStatusQueued)
	if err != nil {
		fmt.Println("Error scaning for next operation to process ", err)
		return nil, fmt.Errorf("Cannot find requested record")
	}
	defer rows.Close()

	var operation Operation
	if rows.Next() {
		rows.Scan(
			&operation.ID,
			&operation.UserId,
			&operation.Type,
			&operation.Status,
			&operation.Payload,
			&operation.Archived,
			&operation.LastOperationAt,
			&operation.CreatedAt,
		)
	} else {
		return nil, nil
	}

	return &operation, nil
}

func initOperationTable() {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS operation (
		id VARCHAR(255) PRIMARY KEY,
		type VARCHAR(50) NOT NULL,
		status VARCHAR(50) NOT NULL,
		payload TEXT,
		archived INT NOT NULL DEFAULT 0,
		user_id TEXT NOT NULL,
		last_operation_at BIGINT,
		created_at BIGINT
	);`)
	if err != nil {
		log.Fatal(err)
	}
}
