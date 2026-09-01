package db

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type DBManager struct {
	mu            sync.RWMutex
	defaultDB     *DB
	projectDBs    map[string]*DB
	projectRoots  map[string]string
	activeProject string
}

func NewDBManager(defaultDBPath string) (*DBManager, error) {
	defaultDB, err := NewDB(defaultDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open default DB: %w", err)
	}
	return &DBManager{
		defaultDB:    defaultDB,
		projectDBs:   make(map[string]*DB),
		projectRoots: make(map[string]string),
	}, nil
}

const (
	DefaultCodraftDir = ".codraft"
	DefaultDBFileName = "tracker.db"
)

func EnsureCodraftDirAndMigrate(rootPath string) string {
	codraftDir := filepath.Join(rootPath, DefaultCodraftDir)
	_ = os.MkdirAll(codraftDir, 0755)

	targetDB := filepath.Join(codraftDir, DefaultDBFileName)
	legacyDB := filepath.Join(rootPath, DefaultDBFileName)

	if _, err := os.Stat(targetDB); os.IsNotExist(err) {
		if _, err := os.Stat(legacyDB); err == nil {
			log.Printf("[DBManager] Migrating DB files from %s to %s", rootPath, codraftDir)
			exts := []string{"", "-wal", "-shm"}
			for _, ext := range exts {
				oldFile := legacyDB + ext
				newFile := targetDB + ext
				if _, err := os.Stat(oldFile); err == nil {
					if err := os.Rename(oldFile, newFile); err != nil {
						log.Printf("[DBManager] Failed to move %s -> %s: %v", oldFile, newFile, err)
					}
				}
			}
		}
	}

	return targetDB
}

func (m *DBManager) SetProjectRoot(project, rootPath string) error {
	if project == "" {
		return fmt.Errorf("project name is empty")
	}

	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return fmt.Errorf("failed to create project root directory %s: %w", rootPath, err)
	}

	dbPath := EnsureCodraftDirAndMigrate(rootPath)

	m.mu.Lock()
	defer m.mu.Unlock()

	if existingRoot, ok := m.projectRoots[project]; ok {
		if existingRoot != rootPath {
			log.Printf("[DBManager] Project %s root changed: %s -> %s", project, existingRoot, rootPath)
		}
	}

	if existingDB, ok := m.projectDBs[project]; ok {
		existingDB.Close()
	}

	db, err := NewDB(dbPath)
	if err != nil {
		log.Printf("[DBManager] Failed to open project DB at %s: %v", dbPath, err)
		return fmt.Errorf("failed to open project DB at %s: %w", dbPath, err)
	}

	m.projectRoots[project] = rootPath
	m.projectDBs[project] = db
	return nil
}

func (m *DBManager) SetActiveProject(project string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeProject = project
}

func (m *DBManager) GetActiveProject() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeProject
}

func (m *DBManager) GetDB(project string) *DB {
	m.mu.RLock()
	targetProject := project
	if targetProject == "" {
		targetProject = m.activeProject
	}

	if targetProject == "" {
		db := m.defaultDB
		m.mu.RUnlock()
		return db
	}

	db, ok := m.projectDBs[targetProject]
	root, hasRoot := m.projectRoots[targetProject]
	m.mu.RUnlock()

	if ok {
		return db
	}

	if hasRoot {
		m.mu.Lock()
		if db, ok := m.projectDBs[targetProject]; ok {
			m.mu.Unlock()
			return db
		}
		dbPath := EnsureCodraftDirAndMigrate(root)
		var err error
		db, err = NewDB(dbPath)
		if err != nil {
			log.Printf("[DBManager] Failed to lazily create DB for project %s at %s: %v", targetProject, dbPath, err)
			m.mu.Unlock()
			return m.defaultDB
		}
		m.projectDBs[targetProject] = db
		m.mu.Unlock()
		return db
	}

	return m.defaultDB
}

func (m *DBManager) GetProjectRoot(project string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if root, ok := m.projectRoots[project]; ok {
		return root
	}
	return ""
}

func (m *DBManager) GetDefaultDB() *DB {
	return m.defaultDB
}

func (m *DBManager) FindPlanDB(planID string) (*DB, *Plan, error) {
	targetDB := m.GetDB("")
	if p, err := targetDB.GetPlan(planID); err == nil {
		return targetDB, p, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, db := range m.projectDBs {
		if db == targetDB {
			continue
		}
		if p, err := db.GetPlan(planID); err == nil {
			return db, p, nil
		}
	}
	if targetDB != m.defaultDB {
		if p, err := m.defaultDB.GetPlan(planID); err == nil {
			return m.defaultDB, p, nil
		}
	}
	return nil, nil, fmt.Errorf("plan not found: %s", planID)
}

func (m *DBManager) FindTaskDB(taskID string) (*DB, *Task, error) {
	targetDB := m.GetDB("")
	if t, err := targetDB.GetTask(taskID); err == nil {
		return targetDB, t, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, db := range m.projectDBs {
		if db == targetDB {
			continue
		}
		if t, err := db.GetTask(taskID); err == nil {
			return db, t, nil
		}
	}
	if targetDB != m.defaultDB {
		if t, err := m.defaultDB.GetTask(taskID); err == nil {
			return m.defaultDB, t, nil
		}
	}
	return nil, nil, fmt.Errorf("task not found: %s", taskID)
}

func (m *DBManager) FindIssueDB(issueID string) (*DB, *Issue, error) {
	targetDB := m.GetDB("")
	if i, err := targetDB.GetIssue(issueID); err == nil {
		return targetDB, i, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, db := range m.projectDBs {
		if db == targetDB {
			continue
		}
		if i, err := db.GetIssue(issueID); err == nil {
			return db, i, nil
		}
	}
	if targetDB != m.defaultDB {
		if i, err := m.defaultDB.GetIssue(issueID); err == nil {
			return m.defaultDB, i, nil
		}
	}
	return nil, nil, fmt.Errorf("issue not found: %s", issueID)
}

func (m *DBManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for project, db := range m.projectDBs {
		if err := db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("project %s: %w", project, err))
		}
	}
	if err := m.defaultDB.Close(); err != nil {
		errs = append(errs, fmt.Errorf("default: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing DBs: %v", errs)
	}
	return nil
}
