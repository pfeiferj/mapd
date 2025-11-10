package cli

import (
	"fmt"

	"capnproto.org/go/capnp/v3"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"pfeifer.dev/mapd/cereal"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	ms "pfeifer.dev/mapd/settings"
)

type downloadModel struct {
	list      list.Model
	path      string
	rootPaths []downloadItem
	state     downloadState
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
	for k := range ms.BOUNDING_BOXES {
		dItem := downloadItem{title: k}
		dItems = append(dItems, dItem)
	}
	items := []list.Item{}
	for _, item := range dItems {
		items = append(items, item)
	}

	listDelegate := list.NewDefaultDelegate()

	m := downloadModel{list: list.New(items, listDelegate, 0, 0)}
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
			for k := range ms.BOUNDING_BOXES[m.path] {
				dItem := downloadItem{title: ms.BOUNDING_BOXES[m.path][k].FullName, desc: k}
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
			arena := capnp.SingleSegment(nil)
			msg, seg, err := capnp.NewMessage(arena)
			if err != nil {
				panic(err)
			}
			evt, err := log.NewRootEvent(seg)
			if err != nil {
				panic(err)
			}
			evt.SetValid(true)
			evt.SetLogMonoTime(cereal.GetTime())
			input, err := evt.NewMapdIn()
			if err != nil {
				panic(err)
			}

			input.SetType(custom.MapdInputType_download)
			path := fmt.Sprintf("%s.%s", m.path, it.desc)
			err = input.SetStr(path)
			if err != nil {
				panic(err)
			}

			data, err := msg.Marshal()
			if err != nil {
				panic(err)
			}
			mm.pub.Send(data)

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
