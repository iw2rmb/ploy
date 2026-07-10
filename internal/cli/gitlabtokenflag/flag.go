package gitlabtokenflag

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

var envNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Resolve(envName string, promptEnabled bool, in io.Reader, promptOut io.Writer) (string, error) {
	envName = strings.TrimSpace(envName)
	if envName != "" && promptEnabled {
		return "", errors.New("--gitlab-token-env and --gitlab-token-prompt are mutually exclusive")
	}
	if envName != "" {
		if !envNameRE.MatchString(envName) {
			return "", errors.New("--gitlab-token-env must be an environment variable name")
		}
		token := strings.TrimSpace(os.Getenv(envName))
		if token == "" {
			return "", fmt.Errorf("--gitlab-token-env %s is not set or empty", envName)
		}
		return token, nil
	}
	if !promptEnabled {
		return "", nil
	}

	token, err := prompt(in, promptOut)
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("--gitlab-token-prompt value is empty")
	}
	return token, nil
}

func prompt(in io.Reader, promptOut io.Writer) (string, error) {
	if promptOut == nil {
		promptOut = io.Discard
	}
	if in == nil {
		in = os.Stdin
	}

	model, programInput, programOptions := newPromptModel(in)
	programOptions = append(programOptions, tea.WithInput(programInput), tea.WithOutput(promptOut))
	program := tea.NewProgram(model, programOptions...)
	finalModel, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("read GitLab token: %w", err)
	}
	if prompt, ok := finalModel.(promptModel); ok {
		if prompt.cancelled {
			return "", errors.New("read GitLab token: cancelled")
		}
		if prompt.input.Err != nil {
			return "", prompt.input.Err
		}
		return prompt.input.Value(), nil
	}
	return "", errors.New("read GitLab token: unexpected prompt state")
}

type promptModel struct {
	input     textinput.Model
	reader    io.Reader
	cancelled bool
}

func newPromptModel(in io.Reader) (promptModel, io.Reader, []tea.ProgramOption) {
	input := textinput.New()
	input.Prompt = "GitLab token: "
	input.EchoMode = textinput.EchoPassword
	input.EchoCharacter = '*'
	_ = input.Focus()
	model := promptModel{input: input}
	if usesInteractiveInput(in) {
		return model, in, nil
	}
	model.reader = in
	return model, nil, []tea.ProgramOption{tea.WithoutRenderer()}
}

func (m promptModel) Init() tea.Cmd {
	if m.reader != nil {
		return readPromptToken(m.reader)
	}
	return nil
}

func (m promptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case promptTokenMsg:
		if msg.err != nil {
			m.input.Err = msg.err
		} else {
			m.input.SetValue(msg.token)
		}
		return m, tea.Quit
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m promptModel) View() tea.View {
	if m.cancelled {
		return tea.NewView("")
	}
	return tea.NewView(m.input.View())
}

type promptTokenMsg struct {
	token string
	err   error
}

func readPromptToken(in io.Reader) tea.Cmd {
	return func() tea.Msg {
		token, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return promptTokenMsg{err: fmt.Errorf("read GitLab token: %w", err)}
		}
		return promptTokenMsg{token: token}
	}
}

func usesInteractiveInput(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
