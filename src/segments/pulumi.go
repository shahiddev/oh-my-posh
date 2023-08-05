package segments

import (
	"github.com/jandedobbeleer/oh-my-posh/src/platform"
	"github.com/jandedobbeleer/oh-my-posh/src/properties"
)

type Pulumi struct {
	props properties.Properties
	env   platform.Environment

	Text string
}

const (
	NewProp properties.Property = "newprop"
)

func (n *Pulumi) Enabled() bool {
	return true
}

func (n *Pulumi) Template() string {
	return " {{.Text}} pulumi2 "

}

func (n *Pulumi) Init(props properties.Properties, env platform.Environment) {
	n.props = props
	n.env = env

	n.Text = props.GetString(NewProp, "Hello")
}
