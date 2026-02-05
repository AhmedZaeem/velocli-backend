package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type LoginDeps struct {
	LoginFunc func(ctx context.Context, licenseKey string) (string, error)
}

type LoginResult struct {
	Token string
}

func RunLogin(deps LoginDeps) (LoginResult, error) {
	m := newLoginModel(deps)
	p := tea.NewProgram(m, tea.WithOutput(os.Stdout))
	finalModel, err := p.Run()
	if err != nil {
		return LoginResult{}, err
	}
	out := finalModel.(loginModel)
	if out.err != nil {
		return LoginResult{}, out.err
	}
	if strings.TrimSpace(out.token) == "" {
		return LoginResult{}, fmt.Errorf("login failed")
	}
	return LoginResult{Token: out.token}, nil
}

type loginResultMsg struct {
	token string
	err   error
}

type loginModel struct {
	deps      LoginDeps
	input     textinput.Model
	spinner   spinner.Model
	submitted bool
	token     string
	err       error
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func newLoginModel(deps LoginDeps) loginModel {
	ti := textinput.New()
	ti.Placeholder = "Paste your license key"
	ti.Focus()
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.CharLimit = 128
	ti.Width = 48

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return loginModel{
		deps:    deps,
		input:   ti,
		spinner: sp,
	}
}

func (m loginModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m loginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.err = fmt.Errorf("cancelled")
			return m, tea.Quit
		case "enter":
			if m.submitted {
				return m, nil
			}
			key := strings.TrimSpace(m.input.Value())
			if key == "" {
				m.err = fmt.Errorf("license key is required")
				return m, tea.Quit
			}
			m.submitted = true
			return m, tea.Batch(m.spinner.Tick, m.loginCmd(key))
		}

	case loginResultMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.token = msg.token
		return m, tea.Quit
	}

	if m.submitted {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m loginModel) View() string {
	if m.submitted && m.err == nil && strings.TrimSpace(m.token) == "" {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render("VeloCli Login"),
			"",
			fmt.Sprintf("%s Authenticating...", m.spinner.View()),
			"",
			helpStyle.Render("Verifying your subscription with the server"),
		) + "\n"
	}
	if m.err != nil {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render("VeloCli Login"),
			"",
			errorStyle.Render("Error: "+m.err.Error()),
		) + "\n"
	}
	if strings.TrimSpace(m.token) != "" {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render("VeloCli Login"),
			"",
			okStyle.Render("Authenticated"),
		) + "\n"
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("VeloCli Login"),
		"",
		labelStyle.Render("License Key"),
		m.input.View(),
		"",
		helpStyle.Render("Enter to continue • Esc to cancel"),
	) + "\n"
}

func (m loginModel) loginCmd(licenseKey string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		token, err := m.deps.LoginFunc(ctx, licenseKey)
		return loginResultMsg{token: token, err: err}
	}
}

