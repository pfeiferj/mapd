package cli

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"pfeifer.dev/mapd/cereal/custom"
	ms "pfeifer.dev/mapd/settings"
)

var settingsList = []list.Item{
	settingsItem{
		title:       "Speed Limit Control Enabled",
		desc:        "When enabled mapd will use the speed limit to determine a suggested speed",
		MessageType: custom.MapdInputType_setSpeedLimitControl,
		Type:        Enable,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.SpeedLimitControlEnabled) },
	},
	settingsItem{
		title:       "Map Curve Speed Control Enabled",
		desc:        "When enabled mapd will use map based curvature calculations to determine a suggested speed",
		MessageType: custom.MapdInputType_setMapCurveSpeedControl,
		Type:        Enable,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.MapCurveSpeedControlEnabled) },
	},
	settingsItem{
		title:       "Vision Curve Speed Control Enabled",
		desc:        "When enabled mapd will use vision model based curvature calculations to determine a suggested speed",
		MessageType: custom.MapdInputType_setVisionCurveSpeedControl,
		Type:        Enable,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.VisionCurveSpeedControlEnabled) },
	},
	settingsItem{
		title:       "External Speed Limit Control Enabled",
		desc:        "When enabled mapd will use fork provided speed limits to determine a suggested speed",
		MessageType: custom.MapdInputType_setExternalSpeedLimitControl,
		Type:        Enable,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.ExternalSpeedLimitControlEnabled) },
	},
	settingsItem{
		title:       "Set Speed Limit Priority",
		desc:        "Sets the prioritization method for available speed limits",
		MessageType: custom.MapdInputType_setSpeedLimitPriority,
		Type:        Options,
		state:       settingsInput,
		options: []list.Item{
			settingsItem{title: "map", value: func() string { return "" }},
			settingsItem{title: "external", value: func() string { return "" }},
			settingsItem{title: "highest", value: func() string { return "" }},
			settingsItem{title: "lowest", value: func() string { return "" }},
		},
		value: func() string { return fmt.Sprintf("%s", ms.Settings.SpeedLimitPriority) },
	},
	settingsItem{
		title:       "Speed Limit Offset",
		desc:        "The offset that gets applied to a speed limit to determine a target speed",
		MessageType: custom.MapdInputType_setSpeedLimitOffset,
		Type:        Speed,
		state:       unitsInput,
		value: func() string {
			val := ms.Settings.SpeedLimitOffset
			mph := ms.MS_TO_MPH * val
			kph := ms.MS_TO_KPH * val
			return fmt.Sprintf("%f m/s, %f mph, %f kph", val, mph, kph)
		},
	},
	settingsItem{
		title:       "Slow Down For Next Speed Limit",
		desc:        "Determines if mapd will try to meet the upcoming speed limit before reaching it when the upcoming speed limit is lower than the current limit",
		MessageType: custom.MapdInputType_setSlowDownForNextSpeedLimit,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.SlowDownForNextSpeedLimit) },
	},
	settingsItem{
		title:       "Speed Up For Next Speed Limit",
		desc:        "Determines if mapd will try to meet the upcoming speed limit before reaching it when the upcoming speed limit is higher than the current limit",
		MessageType: custom.MapdInputType_setSpeedUpForNextSpeedLimit,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.SpeedUpForNextSpeedLimit) },
	},
	settingsItem{
		title:       "Speed Limit Change Requires Accept",
		desc:        "Requires user acceptance of any speed limit changes before activating",
		MessageType: custom.MapdInputType_setSpeedLimitChangeRequiresAccept,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.SpeedLimitChangeRequiresAccept) },
	},
	settingsItem{
		title:       "Press Gas To Accept Speed Limit",
		desc:        "Pressing the gas will accept a speed limit change",
		MessageType: custom.MapdInputType_setPressGasToAcceptSpeedLimit,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.PressGasToAcceptSpeedLimit) },
	},
	settingsItem{
		title:       "Press Gas To Override Speed Limit",
		desc:        "Pressing the gas will override the speed limit to hold the current speed. Resets when the speed limit changes",
		MessageType: custom.MapdInputType_setPressGasToOverrideSpeedLimit,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.PressGasToOverrideSpeedLimit) },
	},
	settingsItem{
		title:       "Adjust Set Speed To Accept Speed Limit",
		desc:        "Adjusting the set speed once in either direction will accept a speed limit change. Additional set speed changes reject the speed limit",
		MessageType: custom.MapdInputType_setAdjustSetSpeedToAcceptSpeedLimit,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.AdjustSetSpeedToAcceptSpeedLimit) },
	},
	settingsItem{
		title:       "Accept Speed Limit Timeout (s)",
		desc:        "The amount of time after a speed limit change is detected that accept inputs will be used. 0 is no limit",
		MessageType: custom.MapdInputType_setAcceptSpeedLimitTimeout,
		Type:        Float,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%f", ms.Settings.AcceptSpeedLimitTimeout) },
	},
	settingsItem{
		title:       "Vision Target Lateral Acceleration (m/s^2)",
		desc:        "The maximum lateral acceleration used in the Vision Curve Control speed calculations",
		MessageType: custom.MapdInputType_setVisionCurveTargetLatA,
		Type:        Float,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%f m/s^2", ms.Settings.VisionCurveTargetLatA) },
	},
	settingsItem{
		title:       "Vision Minimum Target Velocity",
		desc:        "The minimum speed that Vision Curve Control will request to drive",
		MessageType: custom.MapdInputType_setVisionCurveMinTargetV,
		Type:        Speed,
		state:       unitsInput,
		value: func() string {
			val := ms.Settings.VisionCurveMinTargetV
			mph := ms.MS_TO_MPH * val
			kph := ms.MS_TO_KPH * val
			return fmt.Sprintf("%f m/s, %f mph, %f kph", val, mph, kph)
		},
	},
	settingsItem{
		title:       "Mapd Enable Speed",
		desc:        "The speed you can set your cruise control to that will then cause mapd features to engage",
		MessageType: custom.MapdInputType_setEnableSpeed,
		Type:        Speed,
		state:       unitsInput,
		value: func() string {
			val := ms.Settings.EnableSpeed
			mph := ms.MS_TO_MPH * val
			kph := ms.MS_TO_KPH * val
			return fmt.Sprintf("%f m/s, %f mph, %f kph", val, mph, kph)
		},
	},
	settingsItem{
		title:       "Use Enable Speed For Speed Limit",
		desc:        "Determines whether the Mapd Enable Speed controls enabling of Speed Limit Control",
		MessageType: custom.MapdInputType_setSpeedLimitUseEnableSpeed,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.SpeedLimitUseEnableSpeed) },
	},
	settingsItem{
		title:       "Use Enable Speed for Map Curve Speed Control",
		desc:        "Determines whether the Mapd Enable Speed controls enabling of Curve Speed Control",
		MessageType: custom.MapdInputType_setMapCurveUseEnableSpeed,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.MapCurveUseEnableSpeed) },
	},
	settingsItem{
		title:       "Use Enable Speed for Vision Curve Speed Control",
		desc:        "Determines whether the Mapd Enable Speed controls enabling of Vision Curve Speed Control",
		MessageType: custom.MapdInputType_setVisionCurveUseEnableSpeed,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.VisionCurveUseEnableSpeed) },
	},
	settingsItem{
		title:       "Hold Speed Limit While Changing Set Speed",
		desc:        "When enabled mapd will suggest using the speed limit while the cruise control speed is changing. This prevents speeding up while trying to reach the enable speed",
		MessageType: custom.MapdInputType_setHoldSpeedLimitWhileChangingSetSpeed,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.HoldSpeedLimitWhileChangingSetSpeed) },
	},
	settingsItem{
		title:       "Hold Last Seen Speed Limit",
		desc:        "When enabled mapd will use the last seen speed limit if it cannot determine a current speed limit",
		MessageType: custom.MapdInputType_setHoldLastSeenSpeedLimit,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.HoldLastSeenSpeedLimit) },
	},
	settingsItem{
		title:       "Target Speed Jerk (m/s^3)",
		desc:        "The target amount of jerk to use when determining speed change activation distance (map curve and speed limit)",
		MessageType: custom.MapdInputType_setTargetSpeedJerk,
		Type:        Float,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%f m/s^3", ms.Settings.TargetSpeedJerk) },
	},
	settingsItem{
		title:       "Target Speed Accel (m/s^2)",
		desc:        "The target amount of acceleration to use when determining speed change activation distance (map curve and speed limit)",
		MessageType: custom.MapdInputType_setTargetSpeedAccel,
		Type:        Float,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%f m/s^2", ms.Settings.TargetSpeedAccel) },
	},
	settingsItem{
		title:       "Target Speed Time Offset (s)",
		desc:        "An offset for the time before a target position to reach the target speed (map curve and speed limit)",
		MessageType: custom.MapdInputType_setTargetSpeedTimeOffset,
		Type:        Float,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%f s", ms.Settings.TargetSpeedTimeOffset) },
	},
	settingsItem{
		title:       "Map Curve Target Lateral Acceleration (m/s^2)",
		desc:        "The maximum lateral acceleration used in the Map Curve Control speed calculations",
		MessageType: custom.MapdInputType_setMapCurveTargetLatA,
		Type:        Float,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%f m/s^2", ms.Settings.MapCurveTargetLatA) },
	},
	settingsItem{
		title:       "Default Lane Width",
		desc:        "The default lane width to use when determining if we are currently on a road",
		MessageType: custom.MapdInputType_setDefaultLaneWidth,
		Type:        Float,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%f meters", ms.Settings.DefaultLaneWidth) },
	},
	settingsItem{
		title:       "Set Log Level",
		desc:        "Modify how verbose logging will be for the mapd system",
		MessageType: custom.MapdInputType_setLogLevel,
		Type:        Options,
		state:       settingsInput,
		options: []list.Item{
			settingsItem{title: "error", value: func() string { return "" }},
			settingsItem{title: "warn", value: func() string { return "" }},
			settingsItem{title: "info", value: func() string { return "" }},
			settingsItem{title: "debug", value: func() string { return "" }},
		},
		value: func() string { return fmt.Sprintf("%s", ms.Settings.LogLevel) },
	},
	settingsItem{
		title:       "Use JSON Logger",
		desc:        "When true the logs will be output in a json format instead of a text format",
		MessageType: custom.MapdInputType_setLogJson,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.LogJson) },
	},
	settingsItem{
		title:       "Log Source Location",
		desc:        "When true the logs will include the file and line that wrote the log",
		MessageType: custom.MapdInputType_setLogSource,
		Type:        Bool,
		state:       settingsInput,
		value:       func() string { return fmt.Sprintf("%t", ms.Settings.LogSource) },
	},
	settingsItem{
		title: "Load Default Settings",
		desc:  "Loads the default settings",
		state: defaultSettings,
		value: func() string { return "" },
	},
	settingsItem{
		title: "Load Recommended Settings",
		desc:  "Loads the recommended settings",
		state: recommendedSettings,
		value: func() string { return "" },
	},
	settingsItem{
		title: "Save Settings",
		desc:  "Persists any updates to the settings across reboots",
		state: saveSettings,
		value: func() string { return "" },
	},
	settingsItem{
		title: "Return to Main Menu",
		desc:  "Exit settings configuration and return to the initial actions menu",
		state: settingsExit,
		value: func() string { return "" },
	},
}

var enableList = []list.Item{
	settingsItem{
		title: "Enable",
		value: func() string { return "" },
	},
	settingsItem{
		title: "Disable",
		value: func() string { return "" },
	},
}

var boolList = []list.Item{
	settingsItem{
		title: "Yes",
		value: func() string { return "" },
	},
	settingsItem{
		title: "No",
		value: func() string { return "" },
	},
}

var unitsList = []list.Item{
	settingsItem{
		title: "m/s",
		value: func() string { return "" },
	},
	settingsItem{
		title: "mph",
		value: func() string { return "" },
	},
	settingsItem{
		title: "kph",
		value: func() string { return "" },
	},
}

type SettingType int

const (
	String SettingType = iota
	Float
	Bool
	Speed
	Enable
	Options
)

type SpeedUnit int

const (
	Ms SpeedUnit = iota
	Mph
	Kph
)

type settingsState int

const (
	showSettingsMenu settingsState = iota
	settingsExit
	settingsInput
	unitsInput
	saveSettings
	defaultSettings
	recommendedSettings
)

type settingsItem struct {
	title, desc string
	state       settingsState
	MessageType custom.MapdInputType
	Type        SettingType
	options     []list.Item
	value       func() string
}

func (i settingsItem) Title() string {
	val := i.value()
	if val != "" {
		return i.title + " = " + val
	}
	return i.title
}
func (i settingsItem) Description() string { return i.desc }
func (i settingsItem) FilterValue() string { return i.title }

type settingsModel struct {
	list         list.Model
	state        settingsState
	textInput    textinput.Model
	selectedItem settingsItem
	prompt       string
	speedUnit    SpeedUnit
}

func (m settingsModel) Update(msg tea.Msg, mm *uiModel) (settingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && m.state == showSettingsMenu && m.list.FilterState() != list.Filtering {
			it := m.list.SelectedItem().(settingsItem)
			m.selectedItem = it
			m.state = it.state
			switch m.state {
			case settingsExit:
				m.state = showSettingsMenu
				mm.state = showMenu
			case settingsInput:
				if m.selectedItem.Type == Enable {
					m.list.Title = m.selectedItem.Title()
					m.list.SetItems(enableList)
					m.list.ResetSelected()
				}
				if m.selectedItem.Type == Options {
					m.list.Title = m.selectedItem.Title()
					m.list.SetItems(m.selectedItem.options)
					m.list.ResetSelected()
				}
				if m.selectedItem.Type == Bool {
					m.list.Title = m.selectedItem.Title()
					m.list.SetItems(boolList)
					m.list.ResetSelected()
				}
				m.prompt = m.selectedItem.Title()
				m.textInput.Reset()
				m.textInput.Focus()
			case saveSettings:
				m.saveSettings(mm)
			case defaultSettings:
				msg, input := mm.pub.NewMessage(true)

				input.SetType(custom.MapdInputType_loadDefaultSettings)

				err := mm.pub.Send(msg)
				if err != nil {
					panic(err)
				}

				m.saveSettings(mm)
			case recommendedSettings:
				msg, input := mm.pub.NewMessage(true)

				input.SetType(custom.MapdInputType_loadRecommendedSettings)

				err := mm.pub.Send(msg)
				if err != nil {
					panic(err)
				}

				m.saveSettings(mm)
			case unitsInput:
				m.list.SetItems(unitsList)
				m.list.Title = "Select Units"
				m.list.ResetSelected()
			}
			return m, nil
		} else if msg.Type == tea.KeyEnter && m.state == unitsInput && m.list.FilterState() != list.Filtering {
			switch m.list.SelectedItem().(settingsItem).title {
			case "m/s":
				m.speedUnit = Ms
			case "mph":
				m.speedUnit = Mph
			case "kph":
				m.speedUnit = Kph
			}
			m.state = settingsInput
			m.prompt = m.selectedItem.title
			m.textInput.Reset()
			m.textInput.Focus()
		} else if msg.Type == tea.KeyEnter && m.state == settingsInput && m.list.FilterState() != list.Filtering {
			m.state = showSettingsMenu

			msg, input := mm.pub.NewMessage(true)

			input.SetType(m.selectedItem.MessageType)

			result := m.textInput.Value()

			switch m.selectedItem.Type {
			case String:
				err := input.SetStr(result)
				if err != nil {
					panic(err)
				}
			case Bool:
				switch m.list.SelectedItem().(settingsItem).title {
				case "Yes":
					input.SetBool(true)
				case "No":
					input.SetBool(false)
				}
			case Float:
				val, err := strconv.ParseFloat(result, 32)
				if err != nil {
					panic(err)
				}
				input.SetFloat(float32(val))
			case Enable:
				switch m.list.SelectedItem().(settingsItem).title {
				case "Enable":
					input.SetBool(true)
				case "Disable":
					input.SetBool(false)
				}
			case Options:
				err := input.SetStr(m.list.SelectedItem().(settingsItem).title)
				if err != nil {
					panic(err)
				}
			case Speed:
				val, err := strconv.ParseFloat(result, 32)
				if err != nil {
					panic(err)
				}
				switch m.speedUnit {
				case Mph:
					val = val * ms.MPH_TO_MS
				case Kph:
					val = val * ms.KPH_TO_MS
				}
				input.SetFloat(float32(val))
			}
			err := mm.pub.Send(msg)
			if err != nil {
				panic(err)
			}
			m.list.SetItems(settingsList)
			m.list.ResetSelected()
			m.list.Title = "Mapd Settings"
			return m, nil
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		m.textInput.Width = msg.Width - h
		m.textInput.CharLimit = 256
	}

	var cmd tea.Cmd
	switch m.state {
	case settingsInput:
		switch m.selectedItem.Type {
		case Enable:
			m.list, cmd = m.list.Update(msg)
		case Bool:
			m.list, cmd = m.list.Update(msg)
		case Options:
			m.list, cmd = m.list.Update(msg)
		default:
			m.textInput, cmd = m.textInput.Update(msg)
		}
	default:
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

func (m *settingsModel) saveSettings(mm *uiModel) {
	m.state = showSettingsMenu
	mm.state = showMenu

	msg, input := mm.pub.NewMessage(true)

	input.SetType(custom.MapdInputType_saveSettings)

	err := mm.pub.Send(msg)
	if err != nil {
		panic(err)
	}
}

func (m settingsModel) View() string {
	switch m.state {
	case settingsInput:
		switch m.selectedItem.Type {
		case Enable:
			return docStyle.Render(m.list.View())
		case Bool:
			return docStyle.Render(m.list.View())
		case Options:
			return docStyle.Render(m.list.View())
		default:
			return docStyle.Render(fmt.Sprintf(
				"%s\n\n%s\n\n%s",
				m.prompt,
				m.textInput.View(),
				"(esc to quit)",
			) + "\n")
		}
	default:
		return docStyle.Render(m.list.View())
	}
}

func getSettingsModel() settingsModel {
	listDelegate := list.NewDefaultDelegate()
	m := settingsModel{list: list.New(settingsList, listDelegate, 0, 0), textInput: textinput.New()}
	m.list.Title = "Mapd Settings"
	return m
}
