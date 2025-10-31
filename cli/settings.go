package cli

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	"capnproto.org/go/capnp/v3"
)

type SettingType int

const (
	String SettingType = iota
	Float
	Bool
)

type settingsState int
const (
	showSettingsMenu settingsState = iota
	settingsExit
	settingsInput
	saveSettings
)

type settingsItem struct {
	title, desc string
	state settingsState
	MessageType custom.MapdInputType
	Type SettingType

}

func (i settingsItem) Title() string       { return i.title }
func (i settingsItem) Description() string { return i.desc }
func (i settingsItem) FilterValue() string { return i.title }

type settingsModel struct {
	list list.Model
	state settingsState
	textInput textinput.Model
	selectedItem settingsItem
	prompt string
}

func (m settingsModel) Init() tea.Cmd {
    // Just return `nil`, which means "no I/O right now, please."
    return nil
}

func (m settingsModel) Update(msg tea.Msg, mm *uiModel) (settingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && m.state == showSettingsMenu {
			it := m.list.SelectedItem().(settingsItem)
			m.selectedItem = it
			m.state = it.state
			switch m.state {
			case settingsExit:
				m.state = showSettingsMenu
				mm.state = showMenu
			case settingsInput:
				m.prompt = m.selectedItem.Title()
			case saveSettings:
				m.state = showSettingsMenu
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
				input, err := evt.NewMapdIn()
				if err != nil {
					panic(err)
				}

				input.SetType(custom.MapdInputType_saveSettings)

				data, err := msg.Marshal()
				if err != nil {
					panic(err)
				}
				mm.pub.Send(data)

			}
			return m, nil
		}
		if msg.Type == tea.KeyEnter && m.state == settingsInput {
			m.state = showSettingsMenu

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
			input, err := evt.NewMapdIn()
			if err != nil {
				panic(err)
			}

			input.SetType(m.selectedItem.MessageType)

			result := m.textInput.Value()

			switch m.selectedItem.Type {
			case String:
				err := input.SetString_(result)
				if err != nil {
					panic(err)
				}
			case Bool:
				switch result {
				case "true":
					input.SetBool(true)
				case "false":
					input.SetBool(false)
				}
			case Float:
				val, err := strconv.ParseFloat(result, 32)
				if err != nil {
					panic(err)
				}
				input.SetFloat(float32(val))
			}
			data, err := msg.Marshal()
			if err != nil {
				panic(err)
			}
			mm.pub.Send(data)
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

func (m settingsModel) View() string {
	switch m.state {
	case settingsInput:
		return docStyle.Render(fmt.Sprintf(
			"%s\n\n%s\n\n%s",
			m.prompt,
			m.textInput.View(),
			"(esc to quit)",
			) + "\n")
	default:
		return docStyle.Render(m.list.View())
	}
}

func getSettingsModel() settingsModel {
	items := []list.Item{
		settingsItem{
			title: "Speed Limit Control Enabled",
			desc: "When enabled mapd will use the speed limit to determine a suggested speed",
			MessageType: custom.MapdInputType_setSpeedLimitControl,
			Type: Bool,
			state: settingsInput,
		},
		settingsItem{
			title: "Curve Speed Control Enabled",
			desc: "When enabled mapd will use map based curvature calculations to determine a suggested speed",
			MessageType: custom.MapdInputType_setCurveSpeedControl,
			Type: Bool,
			state: settingsInput,
		},
		settingsItem{
			title: "Vision Curve Speed Control Enabled",
			desc: "When enabled mapd will use vision model based curvature calculations to determine a suggested speed",
			MessageType: custom.MapdInputType_setVisionCurveSpeedControl,
			Type: Bool,
			state: settingsInput,
		},
		settingsItem{
			title: "Set Log Level",
			desc: "Modify how verbose logging will be for the mapd system",
			MessageType: custom.MapdInputType_setLogLevel,
			Type: String,
			state: settingsInput,
		},
		settingsItem{
			title: "Speed Limit Offset",
			desc: "The offset that gets applied to a speed limit to determine a target speed",
			MessageType: custom.MapdInputType_setSpeedLimitOffset,
			Type: Float,
			state: settingsInput,
		},
		settingsItem{
			title: "Vision Target Lateral Acceleration",
			desc: "The maximum lateral acceleration used in the Vision Curve Control speed calculations",
			MessageType: custom.MapdInputType_setVtscTargetLatA,
			Type: Float,
			state: settingsInput,
		},
		settingsItem{
			title: "Vision Minimum Target Velocity",
			desc: "The minimum speed that Vision Curve Control will request to drive",
			MessageType: custom.MapdInputType_setVtscMinTargetV,
			Type: Float,
			state: settingsInput,
		},
		settingsItem{
			title: "Mapd Enable Speed",
			desc: "The speed you can set your cruise control to that will then cause mapd features to engage",
			MessageType: custom.MapdInputType_setEnableSpeed,
			Type: Float,
			state: settingsInput,
		},
		settingsItem{
			title: "Use Enable Speed For Speed Limit",
			desc: "Determines whether the Mapd Enable Speed controls enabling of Speed Limit Control",
			MessageType: custom.MapdInputType_setSpeedLimitUseEnableSpeed,
			Type: Bool,
			state: settingsInput,
		},
		settingsItem{
			title: "Use Enable Speed for Curve Speed Control",
			desc: "Determines whether the Mapd Enable Speed controls enabling of Curve Speed Control",
			MessageType: custom.MapdInputType_setCurveUseEnableSpeed,
			Type: Bool,
			state: settingsInput,
		},
		settingsItem{
			title: "Use Enable Speed for Vision Curve Speed Control",
			desc: "Determines whether the Mapd Enable Speed controls enabling of Vision Curve Speed Control",
			MessageType: custom.MapdInputType_setVtscUseEnableSpeed,
			Type: Bool,
			state: settingsInput,
		},
		settingsItem{
			title: "Save Settings",
			desc: "Persists any updates to the settings across reboots",
			state: saveSettings,
		},
		settingsItem{
			title: "Return to Main Menu",
			desc: "Exit settings configuration and return to the initial actions menu",
			state: settingsExit,
		},
	}

	listDelegate := list.NewDefaultDelegate()
	m := settingsModel{list: list.New(items, listDelegate, 0, 0)}
	m.list.Title = "Mapd Settings"
	return m
}
