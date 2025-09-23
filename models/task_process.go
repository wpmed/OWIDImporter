package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type (
	TaskProcessStatus string
	TaskProcessType   string
)

const (
	TaskProcessStatusProcessing         TaskProcessStatus = "processing"
	TaskProcessStatusUploaded           TaskProcessStatus = "uploaded"
	TaskProcessStatusOverwritten        TaskProcessStatus = "overwritten"
	TaskProcessStatusSkipped            TaskProcessStatus = "skipped"
	TaskProcessStatusDescriptionUpdated TaskProcessStatus = "description_updated"
	TaskProcessStatusRetrying           TaskProcessStatus = "retrying"
	TaskProcessStatusFailed             TaskProcessStatus = "failed"
)

const (
	TaskProcessTypeMap     TaskProcessType = "map"
	TaskProcessTypeCountry TaskProcessType = "country"
)

type TaskProcess struct {
	ID        string            `json:"id"`
	Region    string            `json:"region"`
	Type      TaskProcessType   `json:"type"`
	Year      int               `json:"year"`
	Status    TaskProcessStatus `json:"status"`
	TaskId    string            `json:"taskId"`
	FileName  string            `json:"filename"`
	CreatedAt int64             `json:"createdAt"`
	FillData  string            `json:"fillData"`
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
		fill_data TEXT,
		type VARCHAR(10) NOT NULL,
		status VARCHAR(50) NOT NULL,
		task_id TEXT NOT NULL, 
		created_at BIGINT,
		FOREIGN KEY (task_id) REFERENCES task(id)
	);`)
	if err != nil {
		log.Fatal(err)
	}
}

func NewTaskProcess(region string, year int, filename string, status TaskProcessStatus, taskProcessType TaskProcessType, taskId string) (*TaskProcess, error) {
	taskProcess := TaskProcess{
		ID:        uuid.New().String(),
		Region:    region,
		Year:      year,
		Status:    status,
		TaskId:    taskId,
		FileName:  filename,
		Type:      taskProcessType,
		FillData:  "",
		CreatedAt: time.Now().Unix(),
	}
	stmt, err := db.Prepare("INSERT INTO task_process (id, region, year, filename, fill_data, status, type, task_id, created_at) VALUES (?,?,?,?,?,?,?,?,?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	result, err := stmt.Exec(taskProcess.ID, taskProcess.Region, taskProcess.Year, taskProcess.FileName, taskProcess.FillData, taskProcess.Status, taskProcess.Type, taskProcess.TaskId, taskProcess.CreatedAt)
	if err != nil {
		return nil, err
	}
	fmt.Println("CREATED TASK Process", result)

	return &taskProcess, nil
}

func FindTaskProcessByTaskRegionYear(region string, year int, taskId string) (*TaskProcess, error) {
	var tb TaskProcess
	err := db.QueryRow("SELECT id, region, year, status, type, filename, fill_data, task_id, created_at FROM task_process where task_id=? AND region=? AND year=?", taskId, region, year).Scan(&tb.ID, &tb.Region, &tb.Year, &tb.Status, &tb.Type, &tb.FileName, &tb.FillData, &tb.TaskId, &tb.CreatedAt)
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
	stmt, err := db.Prepare("UPDATE task_process SET region=?, year=?, status=?, filename=?, fill_data=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(taskProcess.Region, taskProcess.Year, taskProcess.Status, taskProcess.FileName, taskProcess.FillData, taskProcess.ID)
	if err != nil {
		return err
	}
	fmt.Println("UPDATED TASK Process", result)

	return nil
}

func FindTaskProcessesByTaskId(id string) ([]TaskProcess, error) {
	taskProcesses := make([]TaskProcess, 0)

	rows, err := db.Query("SELECT id, region, year, status, type, filename, fill_data, task_id, created_at FROM task_process where task_id=? ORDER BY created_at DESC", id)
	if err != nil {
		fmt.Println("Error scaning task procresses for task_id ", id, err)
		return taskProcesses, fmt.Errorf("Cannot find requested records")
	}
	defer rows.Close()

	for rows.Next() {
		var task TaskProcess
		err := rows.Scan(&task.ID, &task.Region, &task.Year, &task.Status, &task.Type, &task.FileName, &task.FillData, &task.TaskId, &task.CreatedAt)
		if err != nil {
			fmt.Println("Error parsing task process", err)
		} else {
			taskProcesses = append(taskProcesses, task)
		}

	}

	return taskProcesses, nil
}

func FindTaskProcessesByTaskIdAndRegion(id, region string) ([]TaskProcess, error) {
	taskProcesses := make([]TaskProcess, 0)

	rows, err := db.Query("SELECT id, region, year, status, type, filename, fill_data, task_id, created_at FROM task_process where task_id=? AND region=? ORDER BY created_at DESC", id, region)
	if err != nil {
		fmt.Println("Error scaning task procresses for task_id ", id, region, err)
		return taskProcesses, fmt.Errorf("Cannot find requested records")
	}
	defer rows.Close()

	for rows.Next() {
		var task TaskProcess
		err := rows.Scan(&task.ID, &task.Region, &task.Year, &task.Status, &task.Type, &task.FileName, &task.FillData, &task.TaskId, &task.CreatedAt)
		if err != nil {
			fmt.Println("Error parsing task process", err)
		} else {
			taskProcesses = append(taskProcesses, task)
		}

	}

	return taskProcesses, nil
}
