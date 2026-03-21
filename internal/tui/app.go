package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbletea"
	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/restore"
	"github.com/harish/packrat/internal/storage"
)

type screen int

const (
	screenSnapshots screen = iota
	screenFiles
	screenRestore
	screenProgress
)

type model struct {
	cfg      *config.Config
	storage  storage.StorageBackend
	restorer *restore.Restorer
	stateDB  *backup.StateDB
	screen   screen

	snapshots []*backup.Snapshot
	snapList  list.Model

	selectedSnap *backup.Snapshot
	fileList     list.Model

	selectedFiles []backup.FileEntry
	restoreMsg    string
	err           error
	width, height int
	quitting      bool
}

type snapshotItem struct {
	snap *backup.Snapshot
}

func (i snapshotItem) Title() string {
	return fmt.Sprintf("%s  %s", i.snap.ID, i.snap.Group)
}
func (i snapshotItem) Description() string {
	changed := i.snap.Stats.ChangedFiles + i.snap.Stats.AddedFiles
	return fmt.Sprintf("%s  %d changed  %d total files",
		i.snap.Timestamp.Format("2006-01-02 15:04:05"),
		changed,
		i.snap.Stats.TotalFiles,
	)
}
func (i snapshotItem) FilterValue() string {
	return i.snap.ID + " " + i.snap.Group
}

type fileItem struct {
	entry    backup.FileEntry
	selected bool
}

func (i fileItem) Title() string {
	return i.entry.Path
}
func (i fileItem) Description() string {
	return fmt.Sprintf("[%s]  %d bytes", i.entry.Status, i.entry.Size)
}
func (i fileItem) FilterValue() string {
	return i.entry.Path
}

// Run launches the TUI application.
func Run(cfg *config.Config, store storage.StorageBackend, stateDB *backup.StateDB) error {
	r := restore.NewRestorer(cfg, store, stateDB)

	snapshots, err := r.ListSnapshots("")
	if err != nil {
		return fmt.Errorf("loading snapshots: %w", err)
	}

	var items []list.Item
	for _, s := range snapshots {
		items = append(items, snapshotItem{snap: s})
	}

	delegate := list.NewDefaultDelegate()
	snapList := list.New(items, delegate, 80, 20)
	snapList.Title = "Packrat Restore"
	snapList.SetShowStatusBar(true)
	snapList.SetFilteringEnabled(true)

	m := model{
		cfg:       cfg,
		storage:   store,
		restorer:  r,
		stateDB:   stateDB,
		screen:    screenSnapshots,
		snapshots: snapshots,
		snapList:  snapList,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.snapList.SetSize(msg.Width-4, msg.Height-6)
		if m.screen == screenFiles {
			m.fileList.SetSize(msg.Width-4, msg.Height-6)
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.screen == screenSnapshots {
				m.quitting = true
				return m, tea.Quit
			}
			// Go back
			m.screen = screenSnapshots
			return m, nil

		case "esc":
			if m.screen != screenSnapshots {
				m.screen = screenSnapshots
				return m, nil
			}

		case "enter":
			return m.handleEnter()

		case "r":
			if m.screen == screenFiles && m.selectedSnap != nil {
				return m.startRestore()
			}
			if m.screen == screenSnapshots {
				return m.handleRestore()
			}
		}
	}

	// Update the active list
	var cmd tea.Cmd
	switch m.screen {
	case screenSnapshots:
		m.snapList, cmd = m.snapList.Update(msg)
	case screenFiles:
		m.fileList, cmd = m.fileList.Update(msg)
	}
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	switch m.screen {
	case screenSnapshots:
		return m.viewSnapshots()
	case screenFiles:
		return m.viewFiles()
	case screenRestore:
		return m.viewRestore()
	case screenProgress:
		return m.viewProgress()
	default:
		return ""
	}
}

func (m model) viewSnapshots() string {
	help := helpStyle.Render("[enter] Browse files  [r] Restore  [q] Quit")
	return m.snapList.View() + "\n" + help
}

func (m model) viewFiles() string {
	if m.selectedSnap == nil {
		return "No snapshot selected"
	}
	header := titleStyle.Render(fmt.Sprintf("Snapshot: %s (%s)", m.selectedSnap.ID, m.selectedSnap.Group))
	help := helpStyle.Render("[r] Restore all  [esc] Back  [q] Quit")
	return header + "\n" + m.fileList.View() + "\n" + help
}

func (m model) viewRestore() string {
	if m.restoreMsg != "" {
		return successStyle.Render(m.restoreMsg) + "\n\n" + helpStyle.Render("[esc] Back  [q] Quit")
	}
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err)) + "\n\n" + helpStyle.Render("[esc] Back  [q] Quit")
	}
	return "Restoring..."
}

func (m model) viewProgress() string {
	return "Restoring files..."
}

func (m model) handleEnter() (tea.Model, tea.Cmd) {
	if m.screen == screenSnapshots {
		item, ok := m.snapList.SelectedItem().(snapshotItem)
		if !ok {
			return m, nil
		}
		m.selectedSnap = item.snap

		// Build file list
		var items []list.Item
		for _, f := range item.snap.Files {
			items = append(items, fileItem{entry: f})
		}
		delegate := list.NewDefaultDelegate()
		m.fileList = list.New(items, delegate, m.width-4, m.height-6)
		m.fileList.Title = fmt.Sprintf("Files in %s", item.snap.ID)
		m.screen = screenFiles
	}
	return m, nil
}

func (m model) handleRestore() (tea.Model, tea.Cmd) {
	item, ok := m.snapList.SelectedItem().(snapshotItem)
	if !ok {
		return m, nil
	}
	return m.doRestore(item.snap)
}

func (m model) startRestore() (tea.Model, tea.Cmd) {
	if m.selectedSnap == nil {
		return m, nil
	}
	return m.doRestore(m.selectedSnap)
}

func (m model) doRestore(snap *backup.Snapshot) (tea.Model, tea.Cmd) {
	m.screen = screenRestore
	opts := restore.RestoreOptions{Force: true}

	err := m.restorer.RestoreSnapshot(context.Background(), snap, opts)
	if err != nil {
		m.err = err
	} else {
		m.restoreMsg = fmt.Sprintf("Restored %d files from %s", snap.Stats.TotalFiles, snap.ID)
	}
	return m, nil
}
