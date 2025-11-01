package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"pfeifer.dev/mapd/cereal/custom"
)

type outputModel struct {
	output custom.MapdOut
	valid bool
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
		"name: %s\nsuggested speed: %f\nspeed limit: %f\nnext speed limit: %f\nvtsc speed: %f\ncurve speed: %f",
		roadname,
		m.output.SuggestedSpeed(),
		m.output.SpeedLimit(),
		m.output.NextSpeedLimit(),
		m.output.VtscSpeed(),
		m.output.CurveSpeed(),
		) + "\n")
}
