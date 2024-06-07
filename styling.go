package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

type LogMessage struct {
	Content termenv.Style
}

type model struct {
	logs         []LogMessage
	progress     []int
	totalTasks   []int
	languages    []string
	done         bool
	mu           sync.Mutex
	progressBars []progress.Model
}

type logMsg LogMessage
type progressMsg struct {
	Index int
	Value int
	Total int
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch msg := msg.(type) {
	case logMsg:
		m.logs = append(m.logs, LogMessage{Content: msg.Content})
	case progressMsg:
		m.progress[msg.Index] += msg.Value
		if msg.Total > 0 {
			m.totalTasks[msg.Index] = msg.Total
		}
		progressBar := &m.progressBars[msg.Index]
		percent := float64(m.progress[msg.Index]) / float64(m.totalTasks[msg.Index])
		progressBar.SetPercent(percent)
	case nil:
		m.done = true
	}
	return m, nil
}

func (m *model) renderLogs() string {
	var sb strings.Builder
	for _, log := range m.logs {
		sb.WriteString(log.Content.String() + "\n")
	}
	return sb.String()
}

func (m *model) View() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var sb strings.Builder

	sb.WriteString(m.renderLogs() + "\n")

	for i := range m.progress {
		sb.WriteString(fmt.Sprintf("Language %s: %s\n", m.languages[i], m.progressBars[i].View()))
	}

	return sb.String()
}

func newModel() *model {
	languages := []string{"en", "es", "fr", "po", "it", "de"}
	progressBars := make([]progress.Model, len(languages))
	totalTasks := make([]int, len(languages))

	for i := range progressBars {
		progressBars[i] = progress.New(progress.WithDefaultGradient())
		totalTasks[i] = 1
	}

	return &model{
		logs:         []LogMessage{},
		progress:     make([]int, len(languages)),
		totalTasks:   totalTasks,
		languages:    languages,
		progressBars: progressBars,
	}
}
