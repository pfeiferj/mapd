package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pfeiferj/gomsgq"

	"pfeifer.dev/mapd/cereal"
)

type mainState int

const (
	showMenu mainState = iota
	showSettings
	showDownload
	showOutput
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type TickMsg time.Time

func tickEvery() tea.Cmd {
	return tea.Every(50*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

type uiModel struct {
	list     list.Model
	state    mainState
	settings settingsModel
	output   outputModel
	download downloadModel
	pub      *gomsgq.MsgqPublisher
	sub      *cereal.MapdOutSubscriber
}
type item struct {
	title, desc string
	state       mainState
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

func initialModel() uiModel {
	items := []list.Item{
		item{title: "Settings", desc: "Modify settings of an active instance of mapd", state: showSettings},
		item{title: "Download", desc: "Trigger a download of maps in an active instance of mapd", state: showDownload},
		item{title: "Watch", desc: "Watch the live output from mapd", state: showOutput},
	}

	listDelegate := list.NewDefaultDelegate()
	pub := cereal.GetMapdCliPub()
	sub := cereal.GetMapdOutSub()
	m := uiModel{list: list.New(items, listDelegate, 0, 0), settings: getSettingsModel(), pub: &pub, sub: &sub, download: getDownloadModel()}
	m.list.Title = "Mapd Actions"
	return m
}

func (m uiModel) Init() tea.Cmd {
	// Just return `nil`, which means "no I/O right now, please."
	return tickEvery()
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if msg.Type == tea.KeyEnter && m.state == showMenu {
			it := m.list.SelectedItem().(item)
			m.state = it.state
			return m, nil
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		m.settings, _ = m.settings.Update(msg, &m)
		m.download, _ = m.download.Update(msg, &m)
		m.output, _ = m.output.Update(msg, &m)
	case TickMsg:
		m.output, _ = m.output.Update(msg, &m)
		return m, tickEvery()
	}

	var cmd tea.Cmd
	switch m.state {
	case showSettings:
		m.settings, cmd = m.settings.Update(msg, &m)
	case showOutput:
		m.output, cmd = m.output.Update(msg, &m)
	case showDownload:
		m.download, cmd = m.download.Update(msg, &m)
	default:
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

func (m uiModel) View() string {
	switch m.state {
	case showSettings:
		return m.settings.View()
	case showOutput:
		return m.output.View()
	case showDownload:
		return m.download.View()
	}
	return docStyle.Render(m.list.View())
}

func interactive() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
