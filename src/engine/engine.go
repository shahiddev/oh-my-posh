package engine

import (
	"strings"

	"github.com/jandedobbeleer/oh-my-posh/src/ansi"
	"github.com/jandedobbeleer/oh-my-posh/src/platform"
	"github.com/jandedobbeleer/oh-my-posh/src/shell"
	"github.com/jandedobbeleer/oh-my-posh/src/template"
)

var (
	cycle *ansi.Cycle = &ansi.Cycle{}
)

type Engine struct {
	Config *Config
	Env    platform.Environment
	Writer *ansi.Writer
	Plain  bool

	console           strings.Builder
	currentLineLength int
	rprompt           string
	rpromptLength     int
}

func (e *Engine) write(text string) {
	e.console.WriteString(text)
}

func (e *Engine) string() string {
	text := e.console.String()
	e.console.Reset()
	return text
}

func (e *Engine) canWriteRightBlock(rprompt bool) (int, bool) {
	if rprompt && (len(e.rprompt) == 0 || e.Plain) {
		return 0, false
	}

	consoleWidth, err := e.Env.TerminalWidth()
	if err != nil || consoleWidth == 0 {
		return 0, false
	}

	promptWidth := e.currentLineLength
	availableSpace := consoleWidth - promptWidth

	// spanning multiple lines
	if availableSpace < 0 {
		overflow := promptWidth % consoleWidth
		availableSpace = consoleWidth - overflow
	}

	if rprompt {
		availableSpace -= e.rpromptLength
	}

	promptBreathingRoom := 5
	if rprompt {
		promptBreathingRoom = 30
	}

	canWrite := availableSpace >= promptBreathingRoom

	return availableSpace, canWrite
}

func (e *Engine) writeRPrompt() {
	space, OK := e.canWriteRightBlock(true)
	if !OK {
		return
	}
	e.write(e.Writer.SaveCursorPosition())
	e.write(strings.Repeat(" ", space))
	e.write(e.rprompt)
	e.write(e.Writer.RestoreCursorPosition())
}

func (e *Engine) pwd() {
	// only print when supported
	sh := e.Env.Shell()
	if sh == shell.ELVISH || sh == shell.XONSH {
		return
	}
	// only print when relevant
	if len(e.Config.PWD) == 0 && !e.Config.OSC99 {
		return
	}

	cwd := e.Env.Pwd()
	// Backwards compatibility for deprecated OSC99
	if e.Config.OSC99 {
		e.write(e.Writer.ConsolePwd(ansi.OSC99, "", "", cwd))
		return
	}
	// Allow template logic to define when to enable the PWD (when supported)
	tmpl := &template.Text{
		Template: e.Config.PWD,
		Env:      e.Env,
	}

	pwdType, err := tmpl.Render()
	if err != nil || len(pwdType) == 0 {
		return
	}

	user := e.Env.User()
	host, _ := e.Env.Host()
	e.write(e.Writer.ConsolePwd(pwdType, user, host, cwd))
}

func (e *Engine) newline() {
	// WARP terminal will remove \n from the prompt, so we hack a newline in
	if e.isWarp() {
		e.write(e.Writer.LineBreak())
	} else {
		e.write("\n")
	}
	e.currentLineLength = 0
}

func (e *Engine) isWarp() bool {
	return e.Env.Getenv("TERM_PROGRAM") == "WarpTerminal"
}

func (e *Engine) shouldFill(filler string, remaining, blockLength int) (string, bool) {
	if len(filler) == 0 {
		return "", false
	}

	padLength := remaining - blockLength
	if padLength <= 0 {
		return "", false
	}

	// allow for easy color overrides and templates
	e.Writer.Write("", "", filler)
	filler, lenFiller := e.Writer.String()
	if lenFiller == 0 {
		return "", false
	}

	repeat := padLength / lenFiller
	return strings.Repeat(filler, repeat), true
}

func (e *Engine) getTitleTemplateText() string {
	tmpl := &template.Text{
		Template: e.Config.ConsoleTitleTemplate,
		Env:      e.Env,
	}
	if text, err := tmpl.Render(); err == nil {
		return text
	}
	return ""
}

func (e *Engine) renderBlock(block *Block, cancelNewline bool) {
	defer func() {
		// when in PowerShell, we need to clear the line after the prompt
		// to avoid the background being printed on the next line
		// when at the end of the buffer.
		// See https://github.com/JanDeDobbeleer/oh-my-posh/issues/65
		if e.Env.Shell() == shell.PWSH || e.Env.Shell() == shell.PWSH5 {
			e.write(e.Writer.ClearAfter())
		}
	}()
	// when in bash, for rprompt blocks we need to write plain
	// and wrap in escaped mode or the prompt will not render correctly
	if e.Env.Shell() == shell.BASH && block.Type == RPrompt {
		block.InitPlain(e.Env, e.Config)
	} else {
		block.Init(e.Env, e.Writer)
	}

	if !block.Enabled() {
		return
	}

	// do not print a newline to avoid a leading space
	// when we're printin the first primary prompt in
	// the shell
	if block.Newline && !cancelNewline {
		e.newline()
	}

	switch block.Type {
	// This is deprecated but we leave it in to not break configs
	// It is encouraged to used "newline": true on block level
	// rather than the standalone the linebreak block
	case LineBreak:
		// do not print a newline to avoid a leading space
		// when we're printin the first primary prompt in
		// the shell
		if !cancelNewline {
			return
		}
		e.newline()
	case Prompt:
		if block.VerticalOffset != 0 {
			e.write(e.Writer.ChangeLine(block.VerticalOffset))
		}

		if block.Alignment == Left {
			text, length := block.RenderSegments()
			e.currentLineLength += length
			e.write(text)
			return
		}

		if block.Alignment != Right {
			return
		}

		text, length := block.RenderSegments()
		e.rpromptLength = length

		space, OK := e.canWriteRightBlock(false)
		if OK {
			switch block.Overflow {
			case Break:
				e.newline()
			case Hide:
				// make sure to fill if needed
				if padText, OK := e.shouldFill(block.Filler, space, 0); OK {
					e.write(padText)
				}
				return
			}
		}

		if padText, OK := e.shouldFill(block.Filler, space, length); OK {
			// in this case we can print plain
			e.write(padText)
			e.write(text)
			return
		}

		prompt := strings.Repeat(" ", space-length)
		prompt += text
		e.currentLineLength = 0
		e.write(prompt)
	case RPrompt:
		e.rprompt, e.rpromptLength = block.RenderSegments()
	}
}
