package cli

import (
	"fmt"
	"strconv"

	"github.com/manifoldco/promptui"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/cereal"
	"capnproto.org/go/capnp/v3"
)

type SettingType int

const (
	String SettingType = iota
	Float
	Bool
)

type Setting struct {
	Name string
	MessageType custom.MapdInputType
	Type SettingType
	Options []string
}
type SettingList struct {
	Settings []Setting
}

func (s *Setting) Handle() (msg *capnp.Message) {
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

	input.SetType(s.MessageType)

	result := ""
	if len(s.Options) > 0 {
		prompt := promptui.Select{
			Label: "Select Value",
			Items: s.Options,
		}

		_, result, err = prompt.Run()

		if err != nil {
			panic(err)
		}
	} else if s.Type == Bool {
		prompt := promptui.Select{
			Label: "Select Value",
			Items: []string{"true", "false"},
		}

		_, result, err = prompt.Run()

		if err != nil {
			panic(err)
		}
	} else if s.Type == String {
		prompt := promptui.Prompt{
			Label: "Input Value",
		}

		result, err = prompt.Run()

		if err != nil {
			panic(err)
		}
	} else if s.Type == Float {
		prompt := promptui.Prompt{
			Label: "Input Float Value",
		}

		result, err = prompt.Run()

		if err != nil {
			panic(err)
		}
	}

	switch s.Type {
	case String:
		err = input.SetString_(result)
		if err != nil {
			panic(err)
		}
	case Bool:
		switch result {
		case "enable":
			input.SetBool(true)
		case "disable":
			input.SetBool(false)
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

	return msg
}

var settingsList = SettingList{
	Settings: []Setting{
		{
			Name: "Speed Limit Control Enabled",
			MessageType: custom.MapdInputType_setSpeedLimitControl,
			Type: Bool,
			Options: []string{
				"enable",
				"disable",
			},
		},
		{
			Name: "Curve Speed Control Enabled",
			MessageType: custom.MapdInputType_setCurveSpeedControl,
			Type: Bool,
			Options: []string{
				"enable",
				"disable",
			},
		},
		{
			Name: "Vision Curve Speed Control Enabled",
			MessageType: custom.MapdInputType_setVisionCurveSpeedControl,
			Type: Bool,
			Options: []string{
				"enable",
				"disable",
			},
		},
		{
			Name: "Set Log Level",
			MessageType: custom.MapdInputType_setVisionCurveSpeedControl,
			Type: String,
			Options: []string{
				"error",
				"warn",
				"info",
				"debug",
			},
		},
		{
			Name: "Set Speed Limit Offset",
			MessageType: custom.MapdInputType_setSpeedLimitOffset,
			Type: Float,
		},
		{
			Name: "Set VTSC Target Lateral Acceleration",
			MessageType: custom.MapdInputType_setVtscTargetLatA,
			Type: Float,
		},
		{
			Name: "Set VTSC Minimum Target Velocity",
			MessageType: custom.MapdInputType_setVtscMinTargetV,
			Type: Float,
		},
	},
}

func (s *SettingList) ToItems() []string {
	var items = make([]string, len(s.Settings))
	for i := range len(s.Settings) {
		items[i] = s.Settings[i].Name
	}
	return items
}

func settings() {
	pub := cereal.GetMapdInputPub()

	for {
		prompt := promptui.Select{
			Label: "Select Setting to Update",
			Items: settingsList.ToItems(),
		}

		_, result, err := prompt.Run()

		if err != nil {
			fmt.Printf("Prompt failed %v\n", err)
			return
		}

		for _, setting := range settingsList.Settings {
			if setting.Name == result {
				msg := setting.Handle()
				data, err := msg.Marshal()
				if err != nil {
					panic(err)
				}
				pub.Send(data)
				break
			}
		}
	}

}
