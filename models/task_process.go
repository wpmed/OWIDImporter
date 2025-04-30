package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type TaskProcessStatus string

const (
	TaskProcessStatusProcessing         TaskProcessStatus = "processing"
	TaskProcessStatusUploaded           TaskProcessStatus = "uploaded"
	TaskProcessStatusOverwritten        TaskProcessStatus = "overwritten"
	TaskProcessStatusSkipped            TaskProcessStatus = "skipped"
	TaskProcessStatusDescriptionUpdated TaskProcessStatus = "description_updated"
	TaskProcessStatusRetrying           TaskProcessStatus = "retrying"
	TaskProcessStatusFailed             TaskProcessStatus = "failed"
)

type TaskProcess struct {
	ID        string            `json:"id"`
	Region    string            `json:"region"`
	Year      int               `json:"year"`
	Status    TaskProcessStatus `json:"status"`
	TaskId    string            `json:"taskId"`
	FileName  string            `json:"filename"`
	CreatedAt int64             `json:"createdAt"`
}

func (t *TaskProcess) Value() (driver.Value, error) {
	b, err := json.Marshal(t)
	return string(b), err
}

func initTaskProcessTable() {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS task_process (
		id VARCHAR(255) PRIMARY KEY,
		region TEXT,
		year INT,
		filename TEXT,
		status VARCHAR(50) NOT NULL,
		task_id TEXT NOT NULL, 
		created_at BIGINT,
		FOREIGN KEY (task_id) REFERENCES task(id)
	);`)
	if err != nil {
		log.Fatal(err)
	}
}

func NewTaskProcess(region string, year int, filename string, status TaskProcessStatus, taskId string) (*TaskProcess, error) {
	taskProcess := TaskProcess{
		ID:        uuid.New().String(),
		Region:    region,
		Year:      year,
		Status:    status,
		TaskId:    taskId,
		FileName:  filename,
		CreatedAt: time.Now().Unix(),
	}
	stmt, err := db.Prepare("INSERT INTO task_process (id, region, year, filename, status, task_id, created_at) VALUES (?,?,?,?,?,?,?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	result, err := stmt.Exec(taskProcess.ID, taskProcess.Region, taskProcess.Year, taskProcess.FileName, taskProcess.Status, taskProcess.TaskId, taskProcess.CreatedAt)
	if err != nil {
		return nil, err
	}
	fmt.Println("CREATED TASK Process", result)

	return &taskProcess, nil
}

func FindTaskProcessByTaskRegionYear(region string, year int, taskId string) (*TaskProcess, error) {
	var tb TaskProcess
	err := db.QueryRow("SELECT id, region, year, status, filename, task_id, created_at FROM task_process where task_id=? AND region=? AND year=?", taskId, region, year).Scan(&tb.ID, &tb.Region, &tb.Year, &tb.Status, &tb.FileName, &tb.TaskId, &tb.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &tb, nil
}

func FailProcessingTaskProcesses(taskId string) error {
	stmt, err := db.Prepare("UPDATE task_process SET status=? WHERE task_id=? AND status=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(TaskProcessStatusFailed, taskId, TaskProcessStatusProcessing)
	if err != nil {
		return err
	}
	fmt.Println("UPDATED TASK Process", result)

	return nil
}

func (taskProcess *TaskProcess) Update() error {
	stmt, err := db.Prepare("UPDATE task_process SET region=?, year=?, status=?, filename=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(taskProcess.Region, taskProcess.Year, taskProcess.Status, taskProcess.FileName, taskProcess.ID)
	if err != nil {
		return err
	}
	fmt.Println("UPDATED TASK Process", result)

	return nil
}

func FindTaskProcessesByTaskId(id string) ([]TaskProcess, error) {
	taskProcesses := make([]TaskProcess, 0)

	rows, err := db.Query("SELECT id, region, year, status, filename, task_id, created_at FROM task_process where task_id=? ORDER BY created_at DESC", id)
	if err != nil {
		fmt.Println("Error scaning task procresses for task_id ", id, err)
		return taskProcesses, fmt.Errorf("Cannot find requested records")
	}
	defer rows.Close()

	for rows.Next() {
		var task TaskProcess
		err := rows.Scan(&task.ID, &task.Region, &task.Year, &task.Status, &task.FileName, &task.TaskId, &task.CreatedAt)
		if err != nil {
			fmt.Println("Error parsing task process", err)
		} else {
			taskProcesses = append(taskProcesses, task)
		}

	}

	return taskProcesses, nil
}
