package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	appdb "codraft-mcp/pkg/db"
	appws "codraft-mcp/pkg/ws"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func NewMCPServer(dbm *appdb.DBManager, hub *appws.Hub) *server.MCPServer {
	hooks := &server.Hooks{}
	hooks.AddBeforeInitialize(func(ctx context.Context, id any, req *mcp.InitializeRequest) {
		if req == nil {
			return
		}
		ci := req.Params.ClientInfo
		caps := req.Params.Capabilities
		roots := caps.Roots != nil
		sampling := caps.Sampling != nil
		LogDebug("MCP", "Initialize from client: name=%s version=%s protocol=%s roots=%v sampling=%v extensions=%d experimental=%d",
			ci.Name, ci.Version, req.Params.ProtocolVersion, roots, sampling,
			len(caps.Extensions), len(caps.Experimental))
		LogInfo("MCP", "Client connected: name=%s version=%s roots_capability=%v", ci.Name, ci.Version, roots)
		setClientInfo(ci)
		handleClientHandshake()
	})

	s := server.NewMCPServer(AppName, AppVersion, server.WithHooks(hooks))

	s.AddTool(mcp.NewTool("set_log_level",
		mcp.WithDescription("Set server log level (debug, info, warning, error)"),
		mcp.WithString("level", mcp.Required(), mcp.Description("Log level: debug, info, warning, error")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}
		levelStr, _ := args["level"].(string)
		if levelStr == "" {
			return mcp.NewToolResultError("Field 'level' is required"), nil
		}
		newLevel := SetLogLevelString(levelStr)
		LogInfo("MCP", "Log level updated via set_log_level to %s", newLevel.String())
		return mcp.NewToolResultText(fmt.Sprintf("Log level set to: %s", newLevel.String())), nil
	})

	s.AddTool(mcp.NewTool("open_tracker_ui",
		mcp.WithDescription("Open CoDraft web UI in IDE Simple Browser or external browser."),
		mcp.WithBoolean("force_external", mcp.Description("If true, open in default system browser instead of IDE Simple Browser")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			args = make(map[string]interface{})
		}
		return handleMCPToolCall("open_tracker_ui", args, func() (*mcp.CallToolResult, error) {
			forceExt, _ := args["force_external"].(bool)
			port := getKnownPort()
			if port <= 0 {
				return mcp.NewToolResultError("Failed to obtain current web UI port"), nil
			}

			url := uiURL(port)

			if forceExt {
				openBrowser(port)
				return mcp.NewToolResultText(fmt.Sprintf("Web UI opened in external browser: %s", url)), nil
			}

			writePortFile(port)
			cwd, _ := os.Getwd()
			key := registryOwnKey()
			if key == "" {
				key = registryKey(RegistryClientPending, cwd)
			}
			registryTouch(key)
			return mcp.NewToolResultText(fmt.Sprintf("Web UI opened in IDE Simple Browser: %s", url)), nil
		})
	})

	s.AddTool(mcp.NewTool("add_task",
		mcp.WithDescription("Add a new task to the tracker"),
		mcp.WithString("title", mcp.Required(), mcp.Description("Task title")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}
		return handleMCPToolCall("add_task", args, func() (*mcp.CallToolResult, error) {
			title, _ := args["title"].(string)
			if title == "" {
				return mcp.NewToolResultError("Field 'title' is required"), nil
			}
			db := dbm.GetDB(autoProject())
			id, err := db.AddTask(title, "", "pending")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			hub.BroadcastEvent("task_updated", map[string]interface{}{"task_id": id})
			return mcp.NewToolResultText(fmt.Sprintf("Task created with ID: %s", id)), nil
		})
	})

	s.AddTool(mcp.NewTool("update_task",
		mcp.WithDescription("Update task status"),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID (e.g. task-123)")),
		mcp.WithString("status", mcp.Required(), mcp.Description("Status: needs_approval, pending, in_progress, done, canceled")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}
		return handleMCPToolCall("update_task", args, func() (*mcp.CallToolResult, error) {
			taskID, _ := args["task_id"].(string)
			status, _ := args["status"].(string)
			if taskID == "" || status == "" {
				return mcp.NewToolResultError("Fields 'task_id' and 'status' are required"), nil
			}

			db, _, err := dbm.FindTaskDB(taskID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if proj, err := db.GetTaskProject(taskID); err == nil && proj != "" {
				dbm.GetDefaultDB().UpsertSession(proj)
			}

			if err := db.UpdateTaskStatus(taskID, status); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			hub.BroadcastEvent("task_updated", map[string]interface{}{"task_id": taskID})
			return mcp.NewToolResultText(fmt.Sprintf("Task %s status updated to %s", taskID, status)), nil
		})
	})

	s.AddTool(mcp.NewTool("report_issue",
		mcp.WithDescription("Report an issue/error for a task or plan"),
		mcp.WithString("task_id", mcp.Description("Task ID (optional if plan_id is provided)")),
		mcp.WithString("plan_id", mcp.Description("Plan ID (optional if task_id is provided)")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Issue description")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		taskID, _ := args["task_id"].(string)
		planID, _ := args["plan_id"].(string)
		desc, _ := args["description"].(string)

		if desc == "" {
			return mcp.NewToolResultError("Field 'description' is required"), nil
		}
		if taskID == "" && planID == "" {
			return mcp.NewToolResultError("Either task_id or plan_id must be specified"), nil
		}

		var db *appdb.DB
		if taskID != "" {
			var err error
			db, _, err = dbm.FindTaskDB(taskID)
			if err != nil {
				db = dbm.GetDB(autoProject())
			}
			if proj, err := db.GetTaskProject(taskID); err == nil && proj != "" {
				dbm.GetDefaultDB().UpsertSession(proj)
			}
		} else if planID != "" {
			var err error
			db, _, err = dbm.FindPlanDB(planID)
			if err != nil {
				db = dbm.GetDB(autoProject())
			}
			if proj, err := db.GetPlanProject(planID); err == nil && proj != "" {
				dbm.GetDefaultDB().UpsertSession(proj)
			}
		} else {
			db = dbm.GetDB(autoProject())
		}

		id, err := db.ReportIssue(taskID, planID, desc)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		hub.BroadcastEvent("issue_reported", map[string]interface{}{"issue_id": id, "task_id": taskID, "plan_id": planID})
		return mcp.NewToolResultText(fmt.Sprintf("Issue reported with ID: %s", id)), nil
	})

	s.AddTool(mcp.NewTool("update_issue",
		mcp.WithDescription("Update issue status and fix notes"),
		mcp.WithString("issue_id", mcp.Required(), mcp.Description("Issue ID")),
		mcp.WithString("status", mcp.Required(), mcp.Description("Status: open, in_progress, resolved")),
		mcp.WithString("fix_notes", mcp.Description("Notes on the fix progress")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		issueID, _ := args["issue_id"].(string)
		status, _ := args["status"].(string)
		notes, _ := args["fix_notes"].(string)
		_, hasNotes := args["fix_notes"]
		if issueID == "" || status == "" {
			return mcp.NewToolResultError("Fields 'issue_id' and 'status' are required"), nil
		}
		db, _, err := dbm.FindIssueDB(issueID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := db.UpdateIssue(issueID, status, notes, hasNotes); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		hub.BroadcastEvent("issue_updated", map[string]interface{}{"issue_id": issueID})
		return mcp.NewToolResultText(fmt.Sprintf("Issue %s updated", issueID)), nil
	})

	s.AddTool(mcp.NewTool("get_board_summary",
		mcp.WithDescription("Get complete summary of all tasks, issues, and their statuses"),
		mcp.WithString("plan_id", mcp.Description("Plan ID filter (optional)")),
		mcp.WithString("project", mcp.Description("Project filter (optional)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		planID, _ := args["plan_id"].(string)
		project, _ := args["project"].(string)
		db := dbm.GetDB(project)
		tasks, err := db.GetSummary(planID, project)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, err := json.MarshalIndent(tasks, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	s.AddTool(mcp.NewTool("draft_plan",
		mcp.WithDescription("Create a new plan in draft status. Used by the agent before implementation."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Plan title")),
		mcp.WithString("description", mcp.Description("Plan description")),
		mcp.WithString("project", mcp.Description("Project name for plan isolation (optional)")),
		mcp.WithString("folder", mcp.Description("Plan folder/category (optional)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		title, _ := args["title"].(string)
		desc, _ := args["description"].(string)
		project, _ := args["project"].(string)
		folder, _ := args["folder"].(string)
		if title == "" {
			return mcp.NewToolResultError("Field 'title' is required"), nil
		}

		if project == "" {
			project = autoProject()
		}
		dbm.GetDefaultDB().UpsertSession(project)

		db := dbm.GetDB(project)
		id, err := db.CreatePlan(title, desc, project, folder)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		hub.BroadcastEvent("plan_updated", map[string]interface{}{"plan_id": id})
		return mcp.NewToolResultText(fmt.Sprintf("Plan created with ID: %s", id)), nil
	})

	s.AddTool(mcp.NewTool("update_plan",
		mcp.WithDescription("Update plan status or folder."),
		mcp.WithString("plan_id", mcp.Required(), mcp.Description("Plan ID")),
		mcp.WithString("status", mcp.Description("New status: draft, approved, in_progress, on_hold, review, completed, canceled, rejected")),
		mcp.WithString("folder", mcp.Description("New folder for the plan")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		planID, _ := args["plan_id"].(string)
		status, _ := args["status"].(string)
		folder, hasFolder := args["folder"].(string)
		if planID == "" {
			return mcp.NewToolResultError("Field 'plan_id' is required"), nil
		}

		db, _, err := dbm.FindPlanDB(planID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if status != "" {
			if err := db.UpdatePlanStatus(planID, status); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}
		if hasFolder {
			if err := db.UpdatePlanFolder(planID, folder); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}
		hub.BroadcastEvent("plan_updated", map[string]interface{}{"plan_id": planID, "status": status, "folder": folder})
		return mcp.NewToolResultText(fmt.Sprintf("Plan %s updated successfully", planID)), nil
	})

	s.AddTool(mcp.NewTool("set_active_project",
		mcp.WithDescription("Register active project session. Call at the start of working on a project for session isolation."),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name (e.g. soc-crm-server)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		project, _ := args["project"].(string)
		if project == "" {
			return mcp.NewToolResultError("Field 'project' is required"), nil
		}

		dbm.SetActiveProject(project)

		if err := dbm.GetDefaultDB().UpsertSession(project); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if db := dbm.GetDB(project); db != dbm.GetDefaultDB() {
			db.UpsertSession(project)
		}
		return mcp.NewToolResultText(fmt.Sprintf("Project %s registered as active", project)), nil
	})

	s.AddTool(mcp.NewTool("set_project_root",
		mcp.WithDescription(fmt.Sprintf("Register project root directory to bind DB storage path. DB will be stored in <root_path>/%s/%s.", appdb.DefaultCodraftDir, appdb.DefaultDBFileName)),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name (e.g. soc-crm-server)")),
		mcp.WithString("root_path", mcp.Required(), mcp.Description("Absolute path to project root directory")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		project, _ := args["project"].(string)
		rootPath, _ := args["root_path"].(string)
		if project == "" || rootPath == "" {
			return mcp.NewToolResultError("Fields 'project' and 'root_path' are required"), nil
		}

		if err := dbm.SetProjectRoot(project, rootPath); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		dbm.GetDefaultDB().UpsertSession(project)
		dbm.GetDB(project).UpsertSession(project)

		return mcp.NewToolResultText(fmt.Sprintf("Project %s registered. DB: %s/%s/%s", project, rootPath, appdb.DefaultCodraftDir, appdb.DefaultDBFileName)), nil
	})

	s.AddTool(mcp.NewTool("propose_task",
		mcp.WithDescription("Propose a new task for a plan. Created with status needs_approval."),
		mcp.WithString("plan_id", mcp.Required(), mcp.Description("Plan ID")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Task title")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		planID, _ := args["plan_id"].(string)
		title, _ := args["title"].(string)
		if planID == "" || title == "" {
			return mcp.NewToolResultError("Fields 'plan_id' and 'title' are required"), nil
		}

		db, _, err := dbm.FindPlanDB(planID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if proj, err := db.GetPlanProject(planID); err == nil && proj != "" {
			dbm.GetDefaultDB().UpsertSession(proj)
		}

		id, err := db.AddTask(title, planID, "needs_approval")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		hub.BroadcastEvent("task_updated", map[string]interface{}{"task_id": id, "plan_id": planID})
		return mcp.NewToolResultText(fmt.Sprintf("Task proposed with ID: %s (status: needs_approval)", id)), nil
	})

	s.AddTool(mcp.NewTool("update_plan_task",
		mcp.WithDescription("Edit or delete a task in a plan. Deletion is only allowed for tasks with status needs_approval."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID")),
		mcp.WithString("title", mcp.Description("New task title (for action=edit)")),
		mcp.WithString("action", mcp.Description("Action: edit (default) or delete")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		taskID, _ := args["task_id"].(string)
		action, _ := args["action"].(string)
		title, _ := args["title"].(string)

		if taskID == "" {
			return mcp.NewToolResultError("Field 'task_id' is required"), nil
		}

		db, _, err := dbm.FindTaskDB(taskID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if proj, err := db.GetTaskProject(taskID); err == nil && proj != "" {
			dbm.GetDefaultDB().UpsertSession(proj)
		}

		if action == "" {
			action = "edit"
		}

		if action == "delete" {
			if err := db.DeleteTask(taskID); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			hub.BroadcastEvent("task_updated", map[string]interface{}{"task_id": taskID})
			return mcp.NewToolResultText(fmt.Sprintf("Task %s deleted", taskID)), nil
		}

		if action == "edit" && title != "" {
			if err := db.UpdateTaskTitle(taskID, title); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			hub.BroadcastEvent("task_updated", map[string]interface{}{"task_id": taskID})
			return mcp.NewToolResultText(fmt.Sprintf("Task %s updated", taskID)), nil
		}

		return mcp.NewToolResultError("Specify action=delete or action=edit with title parameter"), nil
	})

	s.AddTool(mcp.NewTool("read_feedback",
		mcp.WithDescription("Read comments (feedback) for a plan or task. If plan_id is specified, returns comments for the plan and all its tasks."),
		mcp.WithString("plan_id", mcp.Description("Plan ID (optional)")),
		mcp.WithString("task_id", mcp.Description("Task ID (optional)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		planID, _ := args["plan_id"].(string)
		taskID, _ := args["task_id"].(string)

		if planID == "" && taskID == "" {
			return mcp.NewToolResultError("Either plan_id or task_id must be specified"), nil
		}

		var db *appdb.DB
		var comments []appdb.Comment
		var err error

		if planID != "" {
			db, _, err = dbm.FindPlanDB(planID)
			if err != nil {
				db = dbm.GetDB(autoProject())
			}
			comments, err = db.GetCommentsForPlan(planID)
		} else {
			db, _, err = dbm.FindTaskDB(taskID)
			if err != nil {
				db = dbm.GetDB(autoProject())
			}
			comments, err = db.GetComments("task", taskID)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		data, err := json.MarshalIndent(comments, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	s.AddTool(mcp.NewTool("add_comment",
		mcp.WithDescription("Add a comment to a plan or task on behalf of AI agent."),
		mcp.WithString("entity_type", mcp.Required(), mcp.Description("Entity type: plan, task, or walkthrough")),
		mcp.WithString("entity_id", mcp.Required(), mcp.Description("Entity ID")),
		mcp.WithString("text", mcp.Required(), mcp.Description("Comment text")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		entityType, _ := args["entity_type"].(string)
		entityID, _ := args["entity_id"].(string)
		text, _ := args["text"].(string)

		if entityType == "" || entityID == "" || text == "" {
			return mcp.NewToolResultError("Fields 'entity_type', 'entity_id', and 'text' are required"), nil
		}

		var db *appdb.DB
		if entityType == "plan" {
			var err error
			db, _, err = dbm.FindPlanDB(entityID)
			if err != nil {
				db = dbm.GetDB(autoProject())
			}
		} else if entityType == "task" {
			var err error
			db, _, err = dbm.FindTaskDB(entityID)
			if err != nil {
				db = dbm.GetDB(autoProject())
			}
		} else {
			db = dbm.GetDB(autoProject())
		}
		id, err := db.AddComment(entityType, entityID, "ai", text)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		hub.BroadcastEvent("comment_added", map[string]interface{}{
			"comment_id":  id,
			"entity_type": entityType,
			"entity_id":   entityID,
		})
		return mcp.NewToolResultText(fmt.Sprintf("Comment added with ID: %s", id)), nil
	})

	s.AddTool(mcp.NewTool("submit_for_walkthrough",
		mcp.WithDescription("Submit plan for review (walkthrough). Transitions plan in_progress->review. Saves git commit hash and agent notes. Only call when work is complete. After calling, ask user to review results in UI."),
		mcp.WithString("plan_id", mcp.Required(), mcp.Description("Plan ID")),
		mcp.WithString("summary_notes", mcp.Required(), mcp.Description("Agent report notes")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		planID, _ := args["plan_id"].(string)
		notes, _ := args["summary_notes"].(string)

		if planID == "" || notes == "" {
			return mcp.NewToolResultError("Fields 'plan_id' and 'summary_notes' are required"), nil
		}

		db, plan, err := dbm.FindPlanDB(planID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if plan.Status != "in_progress" {
			return mcp.NewToolResultError(fmt.Sprintf("Plan must be in in_progress status, current: %s", plan.Status)), nil
		}

		workDir := ""
		if plan.Project != "" {
			workDir = dbm.GetProjectRoot(plan.Project)
		}
		gitHash, err := getGitCommitHash(workDir)
		if err != nil {
			gitHash = "unknown"
		}

		wtID, err := db.CreateWalkthrough(planID, gitHash, notes)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := db.UpdatePlanStatus(planID, "review"); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		hub.BroadcastEvent("walkthrough_submitted", map[string]interface{}{
			"plan_id":        planID,
			"walkthrough_id": wtID,
		})

		result := map[string]string{
			"walkthrough_id":  wtID,
			"git_commit_hash": gitHash,
			"plan_status":     "review",
			"message":         "Plan submitted for review. Please ask user to inspect results in UI and wait for confirmation.",
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	s.AddTool(mcp.NewTool("get_plan",
		mcp.WithDescription("Get detailed plan information: tasks, issues, comments, walkthrough history."),
		mcp.WithString("plan_id", mcp.Required(), mcp.Description("Plan ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		planID, _ := args["plan_id"].(string)
		if planID == "" {
			return mcp.NewToolResultError("Field 'plan_id' is required"), nil
		}

		_, plan, err := dbm.FindPlanDB(planID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	s.AddTool(mcp.NewTool("get_plans",
		mcp.WithDescription("Get summary list of all plans (id, title, status, task counts)."),
		mcp.WithString("project", mcp.Description("Project filter (optional)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		project, _ := args["project"].(string)
		db := dbm.GetDB(project)
		plans, err := db.GetPlans(project)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		data, err := json.MarshalIndent(plans, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	s.AddTool(mcp.NewTool("get_plan_feedback",
		mcp.WithDescription("Get all open issues, comments, plan status, and pending tasks for plan review."),
		mcp.WithString("plan_id", mcp.Required(), mcp.Description("Plan ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Invalid arguments format"), nil
		}

		planID, _ := args["plan_id"].(string)
		if planID == "" {
			return mcp.NewToolResultError("Field 'plan_id' is required"), nil
		}

		db, _, err := dbm.FindPlanDB(planID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fb, err := db.GetPlanFeedback(planID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		data, err := json.MarshalIndent(fb, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	return s
}

func handleMCPToolCall(toolName string, args map[string]interface{}, fn func() (*mcp.CallToolResult, error)) (*mcp.CallToolResult, error) {
	LogDebug("MCP", "Tool call '%s' started with args: %v", toolName, args)
	res, err := fn()
	if err != nil {
		LogError("MCP", "Tool '%s' returned error: %v", toolName, err)
	} else if res != nil && res.IsError {
		LogWarn("MCP", "Tool '%s' validation error: %v", toolName, res.Content)
	} else {
		LogDebug("MCP", "Tool '%s' executed successfully", toolName)
	}
	return res, err
}
