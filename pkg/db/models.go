package db

import "time"

type Task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	PlanID    string    `json:"plan_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Issues    []Issue   `json:"issues,omitempty"`
	Comments  []Comment `json:"comments,omitempty"`
}

type Issue struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	PlanID      string    `json:"plan_id,omitempty"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	FixNotes    string    `json:"fix_notes"`
	CreatedAt   time.Time `json:"created_at"`
}

type Plan struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Status       string        `json:"status"`
	Project      string        `json:"project,omitempty"`
	Folder       string        `json:"folder,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	Tasks        []Task        `json:"tasks,omitempty"`
	Walkthroughs []Walkthrough `json:"walkthroughs,omitempty"`
}

type Comment struct {
	ID         string    `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Author     string    `json:"author"`
	Text       string    `json:"text"`
	CreatedAt  time.Time `json:"created_at"`
}

type Walkthrough struct {
	ID            string    `json:"id"`
	PlanID        string    `json:"plan_id"`
	GitCommitHash string    `json:"git_commit_hash"`
	SummaryNotes  string    `json:"summary_notes"`
	CreatedAt     time.Time `json:"created_at"`
}

type PlanFeedbackIssue struct {
	TaskID      string `json:"task_id"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type PlanFeedback struct {
	PlanID          string              `json:"plan_id"`
	CurrentStatus   string              `json:"current_status"`
	Summary         string              `json:"summary"`
	Issues          []PlanFeedbackIssue `json:"issues"`
	Comments        []Comment           `json:"comments"`
	UnapprovedTasks []Task              `json:"unapproved_tasks"`
}

type Session struct {
	Project      string    `json:"project"`
	StartedAt    time.Time `json:"started_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}
