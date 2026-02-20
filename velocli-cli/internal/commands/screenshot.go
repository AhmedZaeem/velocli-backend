package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var logo = `
╔══════════════════════════════════════════════════════════════╗
║                    ╦╔═╗╔═╗╔╦╗╔═╗╔╗╔╔╦╗╔═╗                 ║
║                    ║║ ║╠╣  ║ ║╣ ║║║ ║ ╚═╗                 ║
║                   ╚╝╚═╝╚   ╩ ╚═╝╝╚╝ ╩ ╚═╝                 ║
║                                                              ║
║          Flutter Development Ecosystem                       ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              ║
║                                                              �n╚══════════════════════════════════════════════════════════════╝
`

type screenshotModel struct {
	frame int
	done  bool
	gif   string
}

func initialScreenshotModel() screenshotModel {
	return screenshotModel{frame: 0}
}

func (m screenshotModel) Init() tea.Cmd {
	return tea.Tick(time.Millisecond*80, func(time.Time) tea.Msg { return tickMsg{} })
}

type tickMsg struct{}

func (m screenshotModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	case tickMsg:
		if m.frame < 60 {
			m.frame++
			return m, tea.Tick(time.Millisecond*80, func(time.Time) tea.Msg { return tickMsg{} })
		}
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m screenshotModel) View() string {
	if m.done {
		return "\n🎉 Screenshot GIF saved to: " + m.gif + "\n"
	}
	header := lipgloss.NewStyle().Foreground(lipgloss.Color("#39FF14")).Bold(true)
	neon := lipgloss.NewStyle().Foreground(lipgloss.Color("#39FF14"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	
	progress := strings.Repeat("█", m.frame/6) + strings.Repeat("░", 10-m.frame/6)
	
	return header.Render("VeloCli Demo Screenshot") + "\n\n" +
		neon.Render(logo) + "\n\n" +
		"Creating animated GIF… " + dim.Render(progress) + "\n\n" +
		dim.Render("Press any key to cancel")
}

func NewScreenshotCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "screenshot",
		Short: "Generate an animated GIF demo of VeloCli",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := tea.NewProgram(initialScreenshotModel())
			m, err := p.Run()
			if err != nil {
				return err
			}
			final := m.(screenshotModel)
			if final.done {
				gifPath := filepath.Join(".", "velocli-demo.gif")
				if err := recordGIF(gifPath); err != nil {
					return err
				}
				final.gif = gifPath
				fmt.Println("\n🎉 Screenshot GIF saved to:", gifPath)
				return nil
			}
			return nil
		},
	}
}

func recordGIF(out string) error {
	if _, err := exec.LookPath("asciinema"); err != nil {
		return fmt.Errorf("asciinema not found: install via 'brew install asciinema' or see https://asciinema.org/docs/installation")
	}
	if _, err := exec.LookPath("agg"); err != nil {
		return fmt.Errorf("agg (asciinema gif generator) not found: install via 'cargo install asciinema-gif' or see https://github.com/asciinema/agg")
	}
	cast := strings.TrimSuffix(out, ".gif") + ".cast"
	rec := exec.Command("asciinema", "rec", "--overwrite", "--quiet", cast)
	rec.Stdout = os.Stdout
	rec.Stderr = os.Stderr
	rec.Stdin = os.Stdin
	if err := rec.Run(); err != nil {
		return fmt.Errorf("asciinema rec failed: %w", err)
	}
	gif := exec.Command("agg", "--speed", "1.5", cast, out)
	if err := gif.Run(); err != nil {
		return fmt.Errorf("agg failed: %w", err)
	}
	_ = os.Remove(cast)
	return nil
}