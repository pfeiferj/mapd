package cli

import (
	"fmt"
	"os"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/pfeiferj/gomsgq"
	"pfeifer.dev/mapd/cereal"
)

type mainState int

const (
	showMenu mainState = iota
	showSettings
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type uiModel struct {
	list list.Model
	state mainState
	settings settingsModel
	pub *gomsgq.MsgqPublisher
}
type item struct {
	title, desc string
	state mainState
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

func initialModel() uiModel {
	items := []list.Item{
		item{title: "Settings", desc: "Modify settings of an active instance of mapd", state: showSettings},
		item{title: "Download", desc: "Trigger a download of maps in an active instance of mapd"},
		item{title: "Watch", desc: "Watch the live output from mapd"},
	}

	listDelegate := list.NewDefaultDelegate()
	pub := cereal.GetMapdInputPub()
	m := uiModel{list: list.New(items, listDelegate, 0, 0), settings: getSettingsModel(), pub: &pub}
	m.list.Title = "Mapd Actions"
	return m
}

func (m uiModel) Init() tea.Cmd {
    // Just return `nil`, which means "no I/O right now, please."
    return nil
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
	}


	var cmd tea.Cmd
	switch m.state {
	case showSettings:
		m.settings, cmd = m.settings.Update(msg, &m)
	default:
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

func (m uiModel) View() string {
	switch m.state {
	case showSettings:
		return m.settings.View()
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
