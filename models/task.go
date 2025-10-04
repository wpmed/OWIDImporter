package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type (
	TaskStatus                    string
	TaskType                      string
	DescriptionOverwriteBehaviour string
)

const (
	TaskStatusQueued      TaskStatus = "queued"
	TaskStatusProcessing  TaskStatus = "processing"
	TaskStatusDone        TaskStatus = "done"
	TaskStatusRetrying    TaskStatus = "retrying"
	TaskStatusSkipped     TaskStatus = "skipped"
	TaskStatusOverwritten TaskStatus = "overwritten"
	TaskStatusFailed      TaskStatus = "failed"
)

const (
	TaskTypeMap   TaskType = "map"
	TaskTypeChart TaskType = "chart"
)

const (
	DescriptionOverwriteBehaviourAll              = "all"
	DescriptionOverwriteBehaviourExceptCategories = "all_except_categories"
	DescriptionOverwriteBehaviourOnlyFile         = "only_file"
)

type Task struct {
	ID                                   string                        `json:"id"`
	UserId                               string                        `json:"userId"`
	URL                                  string                        `json:"url"`
	FileName                             string                        `json:"filename"`
	Description                          string                        `json:"description"`
	DescriptionOverwriteBehaviour        DescriptionOverwriteBehaviour `json:"descriptionOverwriteBehaviour"`
	ImportCountries                      int                           `json:"importCountries"`         // 0 for false, 1 for true
	GenerateTemplateCommons              int                           `json:"generateTemplateCommons"` // 0 for false, 1 for true
	CountryFileName                      string                        `json:"countryFileName"`
	CountryDescription                   string                        `json:"countryDescription"`
	CountryDescriptionOverwriteBehaviour DescriptionOverwriteBehaviour `json:"countryDescriptionOverwriteBehaviour"`
	ChartName                            string                        `json:"chartName"`
	CommonsTemplateName                  string                        `json:"commonsTemplateName"`
	Status                               TaskStatus                    `json:"status"`
	Type                                 TaskType                      `json:"type"`
	LastOperationAt                      int64                         `json:"lastOperationAt"`
	CreatedAt                            int64                         `json:"createdAt"`
}

func (t *Task) Scan(value interface{}) error {
	return json.Unmarshal([]byte(value.(string)), t)
}

func (t *Task) Value() (driver.Value, error) {
	b, err := json.Marshal(t)
	return string(b), err
}

func NewTask(userId, url, fileName, description string, descriptionOverwriteBehaviour DescriptionOverwriteBehaviour, chartName string, status TaskStatus, taskType TaskType, importCountries int, countryFileName, countryDescription string, countryDescriptionOverwriteBehaviour DescriptionOverwriteBehaviour, generateTemplateCommons int) (*Task, error) {
	task := Task{
		ID:                                   uuid.New().String(),
		UserId:                               userId,
		URL:                                  url,
		FileName:                             fileName,
		Description:                          description,
		ChartName:                            chartName,
		DescriptionOverwriteBehaviour:        descriptionOverwriteBehaviour,
		Status:                               status,
		Type:                                 taskType,
		ImportCountries:                      importCountries,
		GenerateTemplateCommons:              generateTemplateCommons,
		CountryFileName:                      countryFileName,
		CountryDescription:                   countryDescription,
		CountryDescriptionOverwriteBehaviour: countryDescriptionOverwriteBehaviour,
		CommonsTemplateName:                  "",
		LastOperationAt:                      time.Now().Unix(),
		CreatedAt:                            time.Now().Unix(),
	}
	stmt, err := db.Prepare("INSERT INTO task (id, user_id, url, file_name, description, description_overwrite_behaviour, chart_name, status, type, import_countries, country_file_name, country_description, country_description_overwrite_behaviour, generate_template_commons, commons_template_name, last_operation_at, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	result, err := stmt.Exec(
		task.ID,
		task.UserId,
		task.URL,
		task.FileName,
		task.Description,
		task.DescriptionOverwriteBehaviour,
		task.ChartName,
		task.Status,
		task.Type,
		task.ImportCountries,
		task.CountryFileName,
		task.CountryDescription,
		task.CountryDescriptionOverwriteBehaviour,
		task.GenerateTemplateCommons,
		task.CommonsTemplateName,
		task.LastOperationAt,
		task.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	fmt.Println("CREATE TASK ", result)

	return &task, nil
}

func (task *Task) Update() error {
	stmt, err := db.Prepare("UPDATE task SET url=?, file_name=?, description=?, description_overwrite_behaviour=?, status=?, import_countries=?, chart_name=?, commons_template_name=?, last_operation_at=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(task.URL, task.FileName, task.Description, task.DescriptionOverwriteBehaviour, task.Status, task.ImportCountries, task.ChartName, task.CommonsTemplateName, task.LastOperationAt, task.ID)
	if err != nil {
		return err
	}
	fmt.Println("UPDATED TASK ", result)

	return nil
}

func (task *Task) Reload() error {
	err := db.QueryRow(
		"SELECT id, user_id, url, file_name, description, description_overwrite_behaviour, chart_name, status, type, import_countries, generate_template_commons, commons_template_name, last_operation_at, created_at FROM task where id=?",
		task.ID,
	).Scan(&task.ID, &task.UserId, &task.URL, &task.FileName, &task.Description, &task.DescriptionOverwriteBehaviour, &task.ChartName, &task.Status, &task.Type, &task.ImportCountries, &task.GenerateTemplateCommons, &task.CommonsTemplateName, &task.LastOperationAt, &task.CreatedAt)
	if err != nil {
		println("Error reloading task for id ", task.ID, err)
		return fmt.Errorf("Error reloading task")
	}

	return nil
}

func UpdateTaskStatus(id string, status TaskStatus) error {
	stmt, err := db.Prepare("UPDATE task SET status=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(status, id)
	if err != nil {
		return err
	}
	fmt.Println("UPDATED TASK status", result)
	return nil
}

func UpdateTaskLastOperationAt(id string) error {
	stmt, err := db.Prepare("UPDATE task SET last_operation_at=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(time.Now().Unix(), id)
	if err != nil {
		return err
	}
	fmt.Println("UPDATED TASK last operation at", result)
	return nil
}

func FindTaskById(id string) (*Task, error) {
	var task Task
	err := db.QueryRow("SELECT id, user_id, url, file_name, description, description_overwrite_behaviour, chart_name, status, type, import_countries, country_file_name, country_description, country_description_overwrite_behaviour, generate_template_commons, commons_template_name, last_operation_at, created_at FROM task where id=?", id).
		Scan(&task.ID,
			&task.UserId,
			&task.URL,
			&task.FileName,
			&task.Description,
			&task.DescriptionOverwriteBehaviour,
			&task.ChartName,
			&task.Status,
			&task.Type,
			&task.ImportCountries,
			&task.CountryFileName,
			&task.CountryDescription,
			&task.CountryDescriptionOverwriteBehaviour,
			&task.GenerateTemplateCommons,
			&task.CommonsTemplateName,
			&task.LastOperationAt,
			&task.CreatedAt,
		)
	if err != nil {
		println("Error scaning for id ", id, err)
		return nil, fmt.Errorf("Cannot find requested record")
	}

	return &task, nil
}

func FindTaskByUserId(id, taskType string) (*[]Task, error) {
	tasks := make([]Task, 0)
	rows, err := db.Query("SELECT id, user_id, url, file_name, description, description_overwrite_behaviour, chart_name, status, type, import_countries, country_file_name, country_description, country_description_overwrite_behaviour, generate_template_commons, commons_template_name, last_operation_at, created_at FROM task where user_id=? AND type=? ORDER BY created_at DESC", id, taskType)
	if err != nil {
		fmt.Println("Error scaning for id ", id, err)
		return nil, fmt.Errorf("Cannot find requested record")
	}

	for rows.Next() {
		fmt.Println(rows.Columns())
		var task Task
		rows.Scan(
			&task.ID,
			&task.UserId,
			&task.URL,
			&task.FileName,
			&task.Description,
			&task.DescriptionOverwriteBehaviour,
			&task.ChartName,
			&task.Status,
			&task.Type,
			&task.ImportCountries,
			&task.CountryFileName,
			&task.CountryDescription,
			&task.CountryDescriptionOverwriteBehaviour,
			&task.GenerateTemplateCommons,
			&task.CommonsTemplateName,
			&task.LastOperationAt,
			&task.CreatedAt,
		)
		tasks = append(tasks, task)
	}

	return &tasks, nil
}

func FindStalledTasks() (*[]Task, error) {
	tasks := make([]Task, 0)
	timeThreshold := time.Now().Unix() - 60*5 // 5 Min threshold
	rows, err := db.Query("SELECT id, user_id, url, file_name, description, description_overwrite_behaviour, chart_name, status, type, last_operation_at, created_at FROM task where status=? AND last_operation_at <= ?", TaskStatusProcessing, timeThreshold)
	for rows.Next() {
		var task Task
		rows.Scan(&task.ID, &task.UserId, &task.URL, &task.FileName, &task.Description, &task.DescriptionOverwriteBehaviour, &task.ChartName, &task.Status, &task.Type, &task.LastOperationAt, &task.CreatedAt)
		tasks = append(tasks, task)
	}
	if err != nil {
		println("Error scaning for stalled tasks: ", err)
		return nil, fmt.Errorf("Cannot find requested record")
	}

	return &tasks, nil
}

func initTaskTable() {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS task (
		id VARCHAR(255) PRIMARY KEY,
		url TEXT,
		file_name TEXT,
		description TEXT,
		description_overwrite_behaviour TEXT,
		chart_name TEXT,
		status VARCHAR(50) NOT NULL,
		import_countries INT,
		country_file_name TEXT,
		country_description TEXT,
		country_description_overwrite_behaviour TEXT,
		commons_template_name TEXT,
		generate_template_commons INT,
		type VARCHAR(10) NOT NULL,
		user_id TEXT NOT NULL,
		last_operation_at BIGINT,
		created_at BIGINT
	);`)
	if err != nil {
		log.Fatal(err)
	}
}
