package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func generateID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UnixMilli(), hex.EncodeToString(b))
}

var validPlanStatuses = map[string]bool{
	"draft": true, "approved": true, "in_progress": true,
	"on_hold": true, "review": true, "completed": true,
	"canceled": true, "rejected": true,
}

var validTaskStatuses = map[string]bool{
	"needs_approval": true, "pending": true, "in_progress": true,
	"done": true, "canceled": true,
}

var validIssueStatuses = map[string]bool{
	"open": true, "in_progress": true, "resolved": true,
}

var planTransitions = map[string]map[string]bool{
	"draft":       {"approved": true, "rejected": true},
	"approved":    {"in_progress": true, "canceled": true, "draft": true},
	"in_progress": {"review": true, "on_hold": true, "canceled": true, "draft": true},
	"on_hold":     {"in_progress": true, "canceled": true, "draft": true},
	"review":      {"completed": true, "in_progress": true, "canceled": true, "draft": true},
	"rejected":    {"draft": true},
	"canceled":    {"draft": true},
}

var taskTransitions = map[string]map[string]bool{
	"needs_approval": {"pending": true, "canceled": true},
	"pending":        {"in_progress": true, "canceled": true},
	"in_progress":    {"done": true, "canceled": true},
}

func validateStatusTransition(transitions map[string]map[string]bool, current, next, entity string) error {
	allowed, ok := transitions[current]
	if !ok {
		return fmt.Errorf("unknown current status for %s: %s", entity, current)
	}
	if !allowed[next] {
		return fmt.Errorf("invalid %s status transition: %s → %s", entity, current, next)
	}
	return nil
}

type DB struct {
	db *sql.DB
}

func NewDB(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Printf("[DB] Failed to open SQLite database at %s: %v", dbPath, err)
		return nil, err
	}

	if err := initSchema(db); err != nil {
		log.Printf("[DB] Failed to initialize schema at %s: %v", dbPath, err)
		return nil, err
	}

	return &DB{db: db}, nil
}

func initSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA busy_timeout=5000;
		PRAGMA foreign_keys=ON;
	`); err != nil {
		log.Printf("[DB] Failed to set PRAGMAs: %v", err)
	}

	query := `
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS issues (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		description TEXT NOT NULL,
		status TEXT NOT NULL,
		fix_notes TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS plans (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		status TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS comments (
		id TEXT PRIMARY KEY,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		author TEXT NOT NULL,
		text TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS walkthroughs (
		id TEXT PRIMARY KEY,
		plan_id TEXT NOT NULL,
		git_commit_hash TEXT NOT NULL,
		summary_notes TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(plan_id) REFERENCES plans(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS sessions (
		project TEXT PRIMARY KEY,
		started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_active_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`

	_, err := db.Exec(query)
	if err != nil {
		return err
	}

	migrations := []string{
		"ALTER TABLE tasks ADD COLUMN plan_id TEXT REFERENCES plans(id) ON DELETE SET NULL",
		"ALTER TABLE issues ADD COLUMN plan_id TEXT REFERENCES plans(id) ON DELETE CASCADE",
		"ALTER TABLE plans ADD COLUMN project TEXT DEFAULT ''",
		"ALTER TABLE plans ADD COLUMN folder TEXT DEFAULT ''",
	}

	for _, m := range migrations {
		if _, merr := db.Exec(m); merr != nil {
			if !strings.Contains(merr.Error(), "duplicate column name") {
				return merr
			}
		}
	}

	return nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) CreatePlan(title, description, project, folder string) (string, error) {
	id := generateID("plan")
	_, err := d.db.Exec("INSERT INTO plans (id, title, description, status, project, folder) VALUES (?, ?, ?, ?, ?, ?)",
		id, title, description, "draft", project, folder)
	return id, err
}

func (d *DB) UpdatePlanStatus(id, status string) error {
	if !validPlanStatuses[status] {
		return fmt.Errorf("invalid plan status: %s", status)
	}

	var current string
	err := d.db.QueryRow("SELECT status FROM plans WHERE id = ?", id).Scan(&current)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("plan not found: %s", id)
		}
		return err
	}

	if err := validateStatusTransition(planTransitions, current, status, "plan"); err != nil {
		return err
	}

	_, err = d.db.Exec("UPDATE plans SET status = ? WHERE id = ?", status, id)
	return err
}

func (d *DB) UpdatePlanFolder(id, folder string) error {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM plans WHERE id = ?", id).Scan(&count)
	if err != nil || count == 0 {
		return fmt.Errorf("plan not found: %s", id)
	}
	_, err = d.db.Exec("UPDATE plans SET folder = ? WHERE id = ?", folder, id)
	return err
}

func (d *DB) DeleteFolder(name string) error {
	if name == "" {
		return nil
	}
	if _, err := d.db.Exec("UPDATE plans SET folder = '' WHERE folder = ?", name); err != nil {
		return err
	}
	val, err := d.GetSetting("custom_folders")
	if err == nil && val != "" {
		var custom []string
		if err := json.Unmarshal([]byte(val), &custom); err == nil {
			newCustom := make([]string, 0, len(custom))
			for _, f := range custom {
				if f != name {
					newCustom = append(newCustom, f)
				}
			}
			newJSON, _ := json.Marshal(newCustom)
			_ = d.SetSetting("custom_folders", string(newJSON))
		}
	}
	return nil
}

func (d *DB) RenameFolder(oldName, newName string) error {
	if newName == "" {
		return d.DeleteFolder(oldName)
	}
	_, err := d.db.Exec("UPDATE plans SET folder = ? WHERE folder = ?", newName, oldName)
	return err
}

func (d *DB) GetFolders() ([]string, error) {
	rows, err := d.db.Query("SELECT DISTINCT folder FROM plans WHERE folder != '' ORDER BY folder")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return folders, nil
}

func (d *DB) GetPlan(id string) (*Plan, error) {
	var p Plan
	var folder sql.NullString
	err := d.db.QueryRow("SELECT id, title, description, status, project, folder, created_at FROM plans WHERE id = ?", id).
		Scan(&p.ID, &p.Title, &p.Description, &p.Status, &p.Project, &folder, &p.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("plan not found: %s", id)
		}
		return nil, err
	}
	if folder.Valid {
		p.Folder = folder.String
	}

	p.Tasks, _ = d.GetSummary(id, "")
	p.Walkthroughs, _ = d.GetWalkthroughs(id)

	return &p, nil
}

func (d *DB) GetPlans(project string) ([]Plan, error) {
	var rows *sql.Rows
	var err error

	if project != "" {
		rows, err = d.db.Query("SELECT id, title, description, status, project, folder, created_at FROM plans WHERE project = ? ORDER BY created_at DESC", project)
	} else {
		rows, err = d.db.Query("SELECT id, title, description, status, project, folder, created_at FROM plans ORDER BY created_at DESC")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var p Plan
		var folder sql.NullString
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.Status, &p.Project, &folder, &p.CreatedAt); err != nil {
			return nil, err
		}
		if folder.Valid {
			p.Folder = folder.String
		}

		var total, done int
		d.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE plan_id = ?", p.ID).Scan(&total)
		d.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE plan_id = ? AND status = 'done'", p.ID).Scan(&done)
		p.Tasks = make([]Task, 1)
		p.Tasks[0] = Task{ID: fmt.Sprintf("%d/%d", done, total)}

		plans = append(plans, p)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return plans, nil
}

func (d *DB) GetPlanSummary(planID string) (*Plan, error) {
	return d.GetPlan(planID)
}

func (d *DB) UpsertSession(project string) error {
	_, err := d.db.Exec(`
		INSERT INTO sessions (project, started_at, last_active_at)
		VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(project) DO UPDATE SET last_active_at = CURRENT_TIMESTAMP`, project)
	return err
}

func (d *DB) GetSessions() ([]Session, error) {
	rows, err := d.db.Query("SELECT project, started_at, last_active_at FROM sessions ORDER BY last_active_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.Project, &s.StartedAt, &s.LastActiveAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (d *DB) GetPlanProject(planID string) (string, error) {
	var project string
	err := d.db.QueryRow("SELECT project FROM plans WHERE id = ?", planID).Scan(&project)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("plan not found: %s", planID)
		}
		return "", err
	}
	return project, nil
}

func (d *DB) GetTaskProject(taskID string) (string, error) {
	var project sql.NullString
	err := d.db.QueryRow(`
		SELECT p.project FROM tasks t
		JOIN plans p ON t.plan_id = p.id
		WHERE t.id = ?`, taskID).Scan(&project)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("task not found or not linked to a plan: %s", taskID)
		}
		return "", err
	}
	if project.Valid {
		return project.String, nil
	}
	return "", nil
}

func (d *DB) SetPlanFolder(planID, folder string) error {
	_, err := d.db.Exec("UPDATE plans SET folder = ? WHERE id = ?", folder, planID)
	return err
}

func (d *DB) GetSetting(key string) (string, error) {
	var value string
	err := d.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

func (d *DB) GetSettings() (map[string]string, error) {
	rows, err := d.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		settings[k] = v
	}
	return settings, rows.Err()
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.db.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (d *DB) GetProjects() ([]string, error) {
	rows, err := d.db.Query("SELECT DISTINCT project FROM plans WHERE project != '' ORDER BY project")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projects, nil
}

func (d *DB) GetPlanFeedback(planID string) (*PlanFeedback, error) {
	fb := &PlanFeedback{PlanID: planID}

	err := d.db.QueryRow("SELECT status FROM plans WHERE id = ?", planID).Scan(&fb.CurrentStatus)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("plan not found: %s", planID)
		}
		return nil, err
	}

	iRows, err := d.db.Query(`
		SELECT task_id, description, status
		FROM issues
		WHERE plan_id = ? OR task_id IN (SELECT id FROM tasks WHERE plan_id = ?)
		ORDER BY created_at DESC`, planID, planID)
	if err != nil {
		return nil, err
	}
	defer iRows.Close()

	var openIssues int
	for iRows.Next() {
		var i PlanFeedbackIssue
		var tid sql.NullString
		if err := iRows.Scan(&tid, &i.Description, &i.Status); err != nil {
			return nil, err
		}
		if tid.Valid {
			i.TaskID = tid.String
		}
		fb.Issues = append(fb.Issues, i)
		if i.Status == "open" {
			openIssues++
		}
	}
	if err := iRows.Err(); err != nil {
		return nil, err
	}

	comments, err := d.GetCommentsForPlan(planID)
	if err != nil {
		return nil, err
	}
	fb.Comments = comments

	tRows, err := d.db.Query("SELECT id, title, status, plan_id, created_at FROM tasks WHERE plan_id = ? AND status = 'needs_approval' ORDER BY created_at DESC", planID)
	if err != nil {
		return nil, err
	}
	defer tRows.Close()

	for tRows.Next() {
		var t Task
		var pid sql.NullString
		if err := tRows.Scan(&t.ID, &t.Title, &t.Status, &pid, &t.CreatedAt); err != nil {
			return nil, err
		}
		if pid.Valid {
			t.PlanID = pid.String
		}
		fb.UnapprovedTasks = append(fb.UnapprovedTasks, t)
	}
	if err := tRows.Err(); err != nil {
		return nil, err
	}

	commentCount := len(fb.Comments)
	if openIssues == 0 && commentCount == 0 {
		fb.Summary = "All tasks approved, no issues found."
	} else {
		fb.Summary = fmt.Sprintf("Found %d open issues and %d comments.", openIssues, commentCount)
	}

	return fb, nil
}

func (d *DB) AddTask(title, planID, status string) (string, error) {
	if status == "" {
		status = "pending"
	}
	if !validTaskStatuses[status] {
		return "", fmt.Errorf("invalid task status: %s", status)
	}

	if planID != "" {
		var exists int
		if err := d.db.QueryRow("SELECT COUNT(*) FROM plans WHERE id = ?", planID).Scan(&exists); err != nil {
			return "", err
		}
		if exists == 0 {
			return "", fmt.Errorf("plan not found: %s", planID)
		}
	}

	id := generateID("task")
	_, err := d.db.Exec("INSERT INTO tasks (id, title, status, plan_id) VALUES (?, ?, ?, ?)",
		id, title, status, nullIfEmpty(planID))
	return id, err
}

func (d *DB) UpdateTaskStatus(id, status string) error {
	if !validTaskStatuses[status] {
		return fmt.Errorf("invalid task status: %s", status)
	}

	var current string
	var planID sql.NullString
	err := d.db.QueryRow("SELECT status, plan_id FROM tasks WHERE id = ?", id).Scan(&current, &planID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("task not found: %s", id)
		}
		return err
	}

	if err := validateStatusTransition(taskTransitions, current, status, "task"); err != nil {
		return err
	}

	if current == "pending" && status == "in_progress" && planID.Valid {
		var pStatus string
		if err := d.db.QueryRow("SELECT status FROM plans WHERE id = ?", planID.String).Scan(&pStatus); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("plan not found: %s", planID.String)
			}
			return err
		}
		if pStatus != "approved" && pStatus != "in_progress" && pStatus != "on_hold" {
			return fmt.Errorf("cannot start task in plan with status %s (need approved/in_progress/on_hold)", pStatus)
		}
	}

	_, err = d.db.Exec("UPDATE tasks SET status = ? WHERE id = ?", status, id)
	return err
}

func (d *DB) GetTask(id string) (*Task, error) {
	var t Task
	var planID sql.NullString
	err := d.db.QueryRow("SELECT id, title, status, plan_id, created_at FROM tasks WHERE id = ?", id).
		Scan(&t.ID, &t.Title, &t.Status, &planID, &t.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found: %s", id)
		}
		return nil, err
	}
	if planID.Valid {
		t.PlanID = planID.String
	}
	return &t, nil
}

func (d *DB) DeleteTask(id string) error {
	var status string
	err := d.db.QueryRow("SELECT status FROM tasks WHERE id = ?", id).Scan(&status)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("task not found: %s", id)
		}
		return err
	}
	if status != "needs_approval" {
		return fmt.Errorf("can only delete tasks with needs_approval status, got: %s", status)
	}
	_, err = d.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	return err
}

func (d *DB) UpdateTaskTitle(id, title string) error {
	_, err := d.db.Exec("UPDATE tasks SET title = ? WHERE id = ?", title, id)
	return err
}

func (d *DB) GetSummary(planID, project string) ([]Task, error) {
	var rows *sql.Rows
	var err error

	if project != "" {
		if planID != "" {
			rows, err = d.db.Query(`
				SELECT t.id, t.title, t.status, t.plan_id, t.created_at
				FROM tasks t
				JOIN plans p ON t.plan_id = p.id
				WHERE p.project = ? AND t.plan_id = ?
				ORDER BY t.created_at DESC`, project, planID)
		} else {
			rows, err = d.db.Query(`
				SELECT t.id, t.title, t.status, t.plan_id, t.created_at
				FROM tasks t
				JOIN plans p ON t.plan_id = p.id
				WHERE p.project = ?
				ORDER BY t.created_at DESC`, project)
		}
	} else if planID != "" {
		rows, err = d.db.Query("SELECT id, title, status, plan_id, created_at FROM tasks WHERE plan_id = ? ORDER BY created_at DESC", planID)
	} else {
		rows, err = d.db.Query("SELECT id, title, status, plan_id, created_at FROM tasks ORDER BY created_at DESC")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var pid sql.NullString
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &pid, &t.CreatedAt); err != nil {
			return nil, err
		}
		if pid.Valid {
			t.PlanID = pid.String
		}

		iRows, err := d.db.Query("SELECT id, task_id, description, status, fix_notes, plan_id, created_at FROM issues WHERE task_id = ?", t.ID)
		if err == nil {
			for iRows.Next() {
				var i Issue
				var ipid sql.NullString
				if err := iRows.Scan(&i.ID, &i.TaskID, &i.Description, &i.Status, &i.FixNotes, &ipid, &i.CreatedAt); err == nil {
					if ipid.Valid {
						i.PlanID = ipid.String
					}
					t.Issues = append(t.Issues, i)
				}
			}
			if err := iRows.Err(); err != nil {
				iRows.Close()
				return nil, err
			}
			iRows.Close()
		}

		t.Comments, _ = d.GetComments("task", t.ID)

		tasks = append(tasks, t)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

func (d *DB) AddComment(entityType, entityID, author, text string) (string, error) {
	id := generateID("comment")
	_, err := d.db.Exec("INSERT INTO comments (id, entity_type, entity_id, author, text) VALUES (?, ?, ?, ?, ?)",
		id, entityType, entityID, author, text)
	return id, err
}

func (d *DB) GetComments(entityType, entityID string) ([]Comment, error) {
	rows, err := d.db.Query("SELECT id, entity_type, entity_id, author, text, created_at FROM comments WHERE entity_type = ? AND entity_id = ? ORDER BY created_at ASC",
		entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.EntityType, &c.EntityID, &c.Author, &c.Text, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return comments, nil
}

func (d *DB) GetCommentsForPlan(planID string) ([]Comment, error) {
	comments, err := d.GetComments("plan", planID)
	if err != nil {
		return nil, err
	}

	taskIDs, err := d.db.Query("SELECT id FROM tasks WHERE plan_id = ?", planID)
	if err != nil {
		return comments, err
	}
	defer taskIDs.Close()

	for taskIDs.Next() {
		var tid string
		if err := taskIDs.Scan(&tid); err != nil {
			continue
		}
		taskComments, _ := d.GetComments("task", tid)
		comments = append(comments, taskComments...)
	}
	if err := taskIDs.Err(); err != nil {
		return nil, err
	}

	return comments, nil
}

func (d *DB) DeleteComment(id string) error {
	_, err := d.db.Exec("DELETE FROM comments WHERE id = ?", id)
	return err
}

func (d *DB) CreateWalkthrough(planID, gitHash, summaryNotes string) (string, error) {
	id := generateID("walkthrough")
	_, err := d.db.Exec("INSERT INTO walkthroughs (id, plan_id, git_commit_hash, summary_notes) VALUES (?, ?, ?, ?)",
		id, planID, gitHash, summaryNotes)
	return id, err
}

func (d *DB) GetWalkthroughs(planID string) ([]Walkthrough, error) {
	rows, err := d.db.Query("SELECT id, plan_id, git_commit_hash, summary_notes, created_at FROM walkthroughs WHERE plan_id = ? ORDER BY created_at DESC", planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var walkthroughs []Walkthrough
	for rows.Next() {
		var w Walkthrough
		if err := rows.Scan(&w.ID, &w.PlanID, &w.GitCommitHash, &w.SummaryNotes, &w.CreatedAt); err != nil {
			return nil, err
		}
		walkthroughs = append(walkthroughs, w)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return walkthroughs, nil
}

func (d *DB) ReportIssue(taskID, planID, description string) (string, error) {
	if taskID == "" && planID == "" {
		return "", fmt.Errorf("at least one of task_id or plan_id must be provided")
	}

	id := generateID("issue")
	_, err := d.db.Exec("INSERT INTO issues (id, task_id, plan_id, description, status) VALUES (?, ?, ?, ?, ?)",
		id, nullIfEmpty(taskID), nullIfEmpty(planID), description, "open")
	return id, err
}

func (d *DB) UpdateIssue(id, status, fixNotes string, hasFixNotes bool) error {
	if !validIssueStatuses[status] {
		return fmt.Errorf("invalid issue status: %s", status)
	}
	if hasFixNotes {
		_, err := d.db.Exec("UPDATE issues SET status = ?, fix_notes = ? WHERE id = ?", status, fixNotes, id)
		return err
	}
	_, err := d.db.Exec("UPDATE issues SET status = ? WHERE id = ?", status, id)
	return err
}

func (d *DB) GetIssue(id string) (*Issue, error) {
	var i Issue
	var tid, pid sql.NullString
	err := d.db.QueryRow("SELECT id, task_id, plan_id, description, status, fix_notes, created_at FROM issues WHERE id = ?", id).
		Scan(&i.ID, &tid, &pid, &i.Description, &i.Status, &i.FixNotes, &i.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("issue not found: %s", id)
		}
		return nil, err
	}
	if tid.Valid {
		i.TaskID = tid.String
	}
	if pid.Valid {
		i.PlanID = pid.String
	}
	return &i, nil
}

func (d *DB) UpdateIssueTaskID(issueID, taskID string) error {
	_, err := d.db.Exec("UPDATE issues SET task_id = ? WHERE id = ?", taskID, issueID)
	return err
}

func (d *DB) UpdateIssueDescription(id, description string) error {
	_, err := d.db.Exec("UPDATE issues SET description = ? WHERE id = ?", description, id)
	return err
}

func (d *DB) AutoHeal(planID, issueDescription string) (issueID, taskID string, err error) {
	var pStatus string
	if err := d.db.QueryRow("SELECT status FROM plans WHERE id = ?", planID).Scan(&pStatus); err != nil {
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("plan not found: %s", planID)
		}
		return "", "", err
	}
	if pStatus != "review" {
		return "", "", fmt.Errorf("auto-heal requires plan in review, got: %s", pStatus)
	}

	issueID, err = d.ReportIssue("", planID, issueDescription)
	if err != nil {
		return "", "", err
	}

	if err := d.UpdatePlanStatus(planID, "in_progress"); err != nil {
		return "", "", err
	}

	taskID, err = d.AddTask("Fix: "+issueDescription, planID, "pending")
	if err != nil {
		return "", "", err
	}

	if err := d.UpdateIssueTaskID(issueID, taskID); err != nil {
		return "", "", err
	}

	return issueID, taskID, nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
