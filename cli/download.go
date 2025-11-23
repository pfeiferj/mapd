package cli

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"pfeifer.dev/mapd/cereal/custom"
	ms "pfeifer.dev/mapd/settings"
)

type downloadModel struct {
	list      list.Model
	path      string
	rootPaths []downloadItem
	state     downloadState
	menu      ms.DownloadMenu
}

type downloadState int

const (
	showRootDownloadMenu downloadState = iota
	showSubDownloadMenu
)

type downloadItem struct {
	title, desc string
}

func (i downloadItem) Title() string {
	return i.title
}

func (i downloadItem) Description() string {
	return i.desc
}
func (i downloadItem) FilterValue() string { return i.title }

func getDownloadModel() downloadModel {
	dItems := []downloadItem{}
	menu := ms.GetDownloadMenu()
	for k := range menu {
		dItem := downloadItem{title: k}
		dItems = append(dItems, dItem)
	}
	items := []list.Item{}
	for _, item := range dItems {
		items = append(items, item)
	}

	listDelegate := list.NewDefaultDelegate()

	m := downloadModel{list: list.New(items, listDelegate, 0, 0), menu: menu}
	m.list.Title = "Select Download Area"
	m.rootPaths = dItems
	return m
}

func (m downloadModel) Update(msg tea.Msg, mm *uiModel) (downloadModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && m.state == showRootDownloadMenu {
			it := m.list.SelectedItem().(downloadItem)
			m.path = it.title

			items := []list.Item{}
			for k := range m.menu[m.path] {
				dItem := downloadItem{title: m.menu[m.path][k].FullName, desc: k}
				items = append(items, dItem)
			}
			m.list.SetItems(items)
			m.list.ResetSelected()

			m.state = showSubDownloadMenu
			return m, nil
		} else if msg.Type == tea.KeyEnter && m.state == showSubDownloadMenu {
			it := m.list.SelectedItem().(downloadItem)
			m.state = showRootDownloadMenu
			mm.state = showMenu
			msg, input := mm.pub.NewMessage(true)

			input.SetType(custom.MapdInputType_download)
			path := fmt.Sprintf("%s.%s", m.path, it.desc)
			err := input.SetStr(path)
			if err != nil {
				panic(err)
			}

			err = mm.pub.Send(msg)
			if err != nil {
				panic(err)
			}

			items := []list.Item{}
			for _, item := range m.rootPaths {
				items = append(items, item)
			}
			m.list.SetItems(items)
			m.list.ResetSelected()
			return m, nil
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m downloadModel) View() string {
	return docStyle.Render(m.list.View())
}
