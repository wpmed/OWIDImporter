package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

// TaskQueuePollInterval is how often the queue checks the database for queued tasks.
const TaskQueuePollInterval = 2 * time.Second

// TaskQueue starts queued tasks with a separate limit per task type, so a long map import
// does not hold up iframe imports and a large iframe batch runs a few at a time.
type TaskQueue struct {
	limits map[models.TaskType]int
	order  []models.TaskType
	run    func(task *models.Task, user *models.User)

	mu      sync.Mutex
	running map[string]models.TaskType
}

// NewTaskQueue runs at most limits[type] tasks of each type at once; types without a
// limit are not started. run does the work and returns when the task is finished.
func NewTaskQueue(limits map[models.TaskType]int, run func(task *models.Task, user *models.User)) *TaskQueue {
	order := []models.TaskType{models.TaskTypeMap, models.TaskTypeIframe}
	for t := range limits {
		if t != models.TaskTypeMap && t != models.TaskTypeIframe {
			order = append(order, t)
		}
	}
	return &TaskQueue{limits: limits, order: order, run: run, running: map[string]models.TaskType{}}
}

// Run checks for queued tasks every interval, forever.
func (q *TaskQueue) Run(interval time.Duration) {
	for {
		q.Dispatch()
		time.Sleep(interval)
	}
}

// Dispatch starts as many queued tasks as the free slots of each type allow.
func (q *TaskQueue) Dispatch() {
	for _, taskType := range q.order {
		for q.runningCount(taskType) < q.limits[taskType] {
			if !q.startNext(taskType) {
				break
			}
		}
	}
}

// startNext claims and starts the oldest queued task of the type. It returns false when
// there is nothing more to start right now.
func (q *TaskQueue) startNext(taskType models.TaskType) bool {
	task, err := models.FindNextQueuedTask(taskType, q.runningIDs())
	if err != nil {
		fmt.Println("Error finding next task to process", err)
		return false
	}
	if task == nil {
		return false
	}
	claimed, err := models.ClaimQueuedTask(task.ID)
	if err != nil {
		fmt.Println("Error claiming task", task.ID, err)
		return false
	}
	if !claimed {
		// Cancelled between the two queries; look again
		return true
	}
	task.Status = models.TaskStatusProcessing
	fmt.Println("Next task: ", task.URL, task.ID)

	user, err := models.FindUserByID(task.UserId)
	if err != nil || user == nil {
		fmt.Println("Can't find user for the task", task.ID, err)
		// Fail the task to get the next
		task.Status = models.TaskStatusFailed
		task.Update()
		utils.SendWSTask(task)
		return true
	}

	q.mu.Lock()
	q.running[task.ID] = taskType
	q.mu.Unlock()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println("Task panicked", task.ID, r)
			}
			q.mu.Lock()
			delete(q.running, task.ID)
			q.mu.Unlock()
		}()
		q.run(task, user)
	}()
	return true
}

func (q *TaskQueue) runningCount(taskType models.TaskType) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, t := range q.running {
		if t == taskType {
			n++
		}
	}
	return n
}

func (q *TaskQueue) runningIDs() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	ids := make([]string, 0, len(q.running))
	for id := range q.running {
		ids = append(ids, id)
	}
	return ids
}

// RunTask runs one claimed task with the service for its type.
func RunTask(task *models.Task, user *models.User) {
	switch task.Type {
	case models.TaskTypeMap:
		fmt.Println("Action message map", task.URL)
		err := StartMap(task.ID, user, StartData{
			Url:                                  task.URL,
			FileName:                             task.FileName,
			Description:                          task.Description,
			DescriptionOverwriteBehaviour:        task.DescriptionOverwriteBehaviour,
			ImportCountries:                      task.ImportCountries == 1,
			CountryFileName:                      task.CountryFileName,
			CountryDescription:                   task.CountryDescription,
			CountryDescriptionOverwriteBehaviour: task.CountryDescriptionOverwriteBehaviour,
			GenerateTemplateCommons:              task.GenerateTemplateCommons == 1,
			TemplateNameFormat:                   task.CommonsTemplateNameFormat,
		})
		if err != nil {
			log.Println("Error starting map", err)
		}
	case models.TaskTypeIframe:
		fmt.Println("Action message iframe", task.URL)
		if err := StartIframe(task.ID, user); err != nil {
			log.Println("Error starting iframe", err)
		}
	}
}
