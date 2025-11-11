package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"pfeifer.dev/mapd/cereal/custom"
)

type outputModel struct {
	output custom.MapdOut
	valid  bool
}

func (m outputModel) Update(msg tea.Msg, mm *uiModel) (outputModel, tea.Cmd) {
	out, success := mm.sub.Read()
	if success {
		m.valid = true
		m.output = out
	}

	return m, nil
}

func (m outputModel) View() string {
	if !m.valid {
		return ""
	}
	roadname, _ := m.output.RoadName()
	return docStyle.Render(fmt.Sprintf(
		"name: %s\nsuggested speed: %f\nspeed limit: %f\nnext speed limit: %f\nnext speed limit distance: %f\nvision curve speed: %f\ncurve speed: %f\ndistance from center: %f\nlanes: %d\nselection type: %s",
		roadname,
		m.output.SuggestedSpeed(),
		m.output.SpeedLimit(),
		m.output.NextSpeedLimit(),
		m.output.NextSpeedLimitDistance(),
		m.output.VisionCurveSpeed(),
		m.output.CurveSpeed(),
		m.output.DistanceFromWayCenter(),
		m.output.Lanes(),
		m.output.WaySelectionType().String(),
	) + "\n")
}
