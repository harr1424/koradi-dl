package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
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
	viewport     viewport.Model
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
	case tea.MouseMsg:
		switch msg.Type {
		case tea.MouseWheelUp:
			m.viewport.LineUp(1)
		case tea.MouseWheelDown:
			m.viewport.LineDown(1)
		}
	case logMsg:
		m.logs = append(m.logs, LogMessage{Content: msg.Content})
		m.viewport.SetContent(m.renderLogs()) // Update viewport content
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

	sb.WriteString(m.viewport.View() + "\n")

	for i := range m.progress {
		percentage := 0
		if m.totalTasks[i] > 0 {
			percentage = int(float64(m.progress[i]) / float64(m.totalTasks[i]) * 100)
		}
		sb.WriteString(fmt.Sprintf("Language %s: %s %d%%\n", m.languages[i], m.progressBars[i].View(), percentage))
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

	vp := viewport.New(80, 10)
	vp.YPosition = 0
	vp.SetContent("")

	return &model{
		logs:         []LogMessage{},
		progress:     make([]int, len(languages)),
		totalTasks:   totalTasks,
		languages:    languages,
		progressBars: progressBars,
		viewport:     vp,
	}
}
