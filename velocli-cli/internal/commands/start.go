package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// --- UI palette ---
var (
	colorAccent    = lipgloss.Color("#A78BFA")   // soft purple
	colorNeon      = lipgloss.Color("#39FF14")   // neon green
	colorMuted     = lipgloss.Color("#6B7280")   // gray
	colorWarn      = lipgloss.Color("#FBBF24")   // amber
	colorBg        = lipgloss.Color("#111827")   // dark slate
	colorHighlight = lipgloss.Color("#F472B6")   // pink
	colorError     = lipgloss.Color("#EF4444")   // red
)

var (
	styleHeader      = lipgloss.NewStyle().Foreground(colorNeon).Bold(true).PaddingBottom(1)
	styleLabel       = lipgloss.NewStyle().Foreground(colorAccent).Bold(true) // Removed MarginTop for better layout control
	styleDim         = lipgloss.NewStyle().Foreground(colorMuted)
	styleWarn        = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	styleError       = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	styleCursor      = lipgloss.NewStyle().Foreground(colorHighlight).Bold(true)
	styleSelected    = lipgloss.NewStyle().Foreground(colorNeon)
	styleCategory    = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).PaddingLeft(1)
	styleKeyHint     = lipgloss.NewStyle().Foreground(colorMuted).MarginTop(1).MarginBottom(1)
	styleBox         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(1, 2)
	styleActiveBox   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorNeon).Padding(1, 2)
)

type SelectionMode string

const (
	SelectionModeMulti  SelectionMode = "multi"
	SelectionModeSingle SelectionMode = "single"
)

type Catalog struct {
	Categories    []Category     `json:"categories"`
	Blocks        []Block        `json:"blocks"`
	MainTemplates []MainTemplate `json:"mainTemplates"`
}

var Version = "0.1.0"

type Category struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	SelectionMode SelectionMode `json:"selectionMode"`
}

type Block struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	CategoryID  string            `json:"categoryId"`
	Description string            `json:"description"`
	BasePath    string            `json:"basePath"`
	Conflicts   []string          `json:"conflicts"`
	Deps        map[string]string `json:"deps"`
	MainTarget  string            `json:"mainTarget"`
	MainMode    string            `json:"mainMode"`
	MainContent string            `json:"mainContent"`
	BlobID      string            `json:"blobId"`
	UpdatedAt   string            `json:"updatedAt"`
}

type MainTemplate struct {
	ID      string            `json:"id"`
	Label   string            `json:"label"`
	Content string            `json:"content"`
	BlobID  string            `json:"blobId"`
	Deps    map[string]string `json:"deps"`
}

type Cursor struct {
	Category int
	Block    int
}

type screen string

const (
	screenSplash     screen = "splash"
	screenLoading    screen = "loading"
	screenProject    screen = "project"
	screenPlatforms  screen = "platforms"
	screenSelect     screen = "select"
	screenTemplate   screen = "template"
	screenPreflight  screen = "preflight"
	screenInstall    screen = "install"
	screenGenerating screen = "generating"
)

var platformKeys = []string{"android", "ios", "web", "linux", "macos", "windows"}

type ideChoice int

const (
	ideNone ideChoice = iota
	ideVSCode
	ideAndroidStudio
)

type generatorOptions struct {
	OutputDir string
	IDE       ideChoice
}

type startModel struct {
	apiURL      string
	catalogURL  string
	catalogPath string
	ignoreEnv   bool

	stream *catalogStream

	screen       screen
	loadErrText  string
	catalogErr   string
	splashStart  time.Time
	splashFrame  int
	catalogReady bool

	categories     []Category
	blocks         []Block
	templates      []MainTemplate
	selectedBlocks map[string]bool
	cursor         Cursor
	focusIdx       int
	projectName    textinput.Model
	description    textinput.Model
	pkgOrOrg       textinput.Model
	descTouched    bool
	pkgTouched     bool
	platforms      map[string]bool
	platformCursor int
	templateIdx    int
	ide            ideChoice
	outputDir      string

	// Flutter/FVM state
	flutterPath      string
	fvmPath          string
	installConfirmed bool
}

func NewStartCmd() *cobra.Command {
	var apiURL string
	var catalogURL string
	var catalogPath string
	var outputDir string
	var ide string
	var ignoreEnv bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Interactive Flutter project generator",
		RunE: func(cmd *cobra.Command, args []string) error {
			if apiURL == "" {
				apiURL = envDefault("VELOCLI_API_URL", "http://localhost:9999")
			}
			if catalogURL == "" {
				catalogURL = envDefault("VELOCLI_CATALOG_URL", "")
			}
			if catalogPath == "" {
				catalogPath = envDefault("VELOCLI_CATALOG_PATH", "")
			}
			if outputDir == "" {
				outputDir = envDefault("VELOCLI_OUTPUT_DIR", "")
			}

			var opts generatorOptions
			switch ide {
			case "vscode":
				opts.IDE = ideVSCode
			case "android-studio":
				opts.IDE = ideAndroidStudio
			default:
				opts.IDE = ideNone
			}
			opts.OutputDir = outputDir

			stream := newCatalogStream()
			model := initialStartModel(apiURL, catalogURL, catalogPath, ignoreEnv, stream, opts)
			p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
			final, err := p.Run()
			if err != nil {
				return err
			}

			m := final.(startModel)
			if m.screen == screenGenerating {
				return runGeneration(m, opts)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&apiURL, "api-url", "", "Backend base URL")
	cmd.Flags().StringVar(&catalogURL, "catalog-url", "", "Full catalog URL (overrides api-url)")
	cmd.Flags().StringVar(&catalogPath, "catalog-path", "", "Catalog path (appended to api-url)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory where projects are created")
	cmd.Flags().StringVar(&ide, "ide", "none", "Open IDE after generation: none|vscode|android-studio")
	cmd.Flags().BoolVar(&ignoreEnv, "ignore-env", false, "Ignore VELOCLI_CATALOG_URL and VELOCLI_CATALOG_PATH env vars")
	return cmd
}

func initialStartModel(apiURL, catalogURL, catalogPath string, ignoreEnv bool, stream *catalogStream, opts generatorOptions) startModel {
	pn := textinput.New()
	pn.Placeholder = "my_app"
	pn.Focus()
	pn.CharLimit = 64
	pn.Width = 50

	desc := textinput.New()
	desc.Placeholder = "A beautiful Flutter app"
	desc.CharLimit = 120
	desc.Width = 50

	pkg := textinput.New()
	pkg.Placeholder = "com.company.my_app"
	pkg.CharLimit = 120
	pkg.Width = 50

	platforms := map[string]bool{
		"android": true,
		"ios":     true,
		"web":     false,
		"linux":   false,
		"macos":   false,
		"windows": false,
	}

	return startModel{
		apiURL:         apiURL,
		catalogURL:     catalogURL,
		catalogPath:    catalogPath,
		ignoreEnv:      ignoreEnv,
		stream:         stream,
		screen:         screenSplash,
		splashStart:    time.Now(),
		projectName:    pn,
		description:    desc,
		pkgOrOrg:       pkg,
		platforms:      platforms,
		selectedBlocks: make(map[string]bool),
		outputDir:      opts.OutputDir,
	}
}

func NewStartModelPlain(apiURL, catalogURL, catalogPath string, ignoreEnv bool) startModel {
	// Initialize with defaults but no stream/UI loop specifics needed
	pn := textinput.New()
	pn.SetValue("my_app")
	
	desc := textinput.New()
	desc.SetValue("A beautiful Flutter app")
	
	pkg := textinput.New()
	pkg.SetValue("com.company.my_app")

	return startModel{
		apiURL:         apiURL,
		catalogURL:     catalogURL,
		catalogPath:    catalogPath,
		ignoreEnv:      ignoreEnv,
		projectName:    pn,
		description:    desc,
		pkgOrOrg:       pkg,
		selectedBlocks: make(map[string]bool),
	}
}

func ParseIDE(s string) ideChoice {
	switch strings.ToLower(s) {
	case "vscode", "code":
		return ideVSCode
	case "android-studio", "studio":
		return ideAndroidStudio
	default:
		return ideNone
	}
}

func (m startModel) Init() tea.Cmd {
	return tea.Batch(
		fetchCatalogCmd(m.apiURL, m.catalogURL, m.catalogPath, m.ignoreEnv),
		startCatalogStreamCmd(m.stream, m.apiURL),
		waitCatalogStreamCmd(m.stream),
		splashTickCmd(),
		checkFlutterCmd(),
	)
}

func (m startModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case catalogLoadedMsg:
		m.categories = msg.catalog.Categories
		m.blocks = msg.catalog.Blocks
		m.templates = msg.catalog.MainTemplates
		m.purgeMissingSelections()
		m.clampCursor()
		m.catalogReady = true
		m.catalogErr = ""
		if msg.updateAvailable && strings.TrimSpace(msg.latestVersion) != "" {
			m.catalogErr = fmt.Sprintf("Update available: %s. Run brew upgrade velocli?", msg.latestVersion)
		}
		if m.screen == screenLoading {
			m.screen = screenProject
			m.syncFocus()
		}
		return m, nil

	case catalogLoadFailedMsg:
		m.catalogErr = msg.err.Error()
		if strings.Contains(strings.ToLower(m.catalogErr), "upgrade required") {
			m.catalogErr = "Upgrade required. Please update VeloCLI to the latest version."
		}
		m.catalogReady = false
		return m, nil

	case catalogStreamMsg:
		if msg.ev.kind == "catalog" {
			m.catalogErr = "Catalog updated. Refreshing…"
			return m, tea.Batch(
				fetchCatalogCmd(m.apiURL, m.catalogURL, m.catalogPath, m.ignoreEnv),
				waitCatalogStreamCmd(m.stream),
			)
		}
		return m, waitCatalogStreamCmd(m.stream)

	case flutterCheckMsg:
		m.flutterPath = msg.flutterPath
		m.fvmPath = msg.fvmPath
		return m, nil

	case splashTickMsg:
		if m.screen != screenSplash {
			return m, nil
		}
		m.splashFrame++
		if time.Since(m.splashStart) > 1600*time.Millisecond {
			if m.catalogReady {
				m.screen = screenProject
				m.syncFocus()
			} else {
				m.screen = screenLoading
			}
			return m, nil
		}
		return m, splashTickCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.screen != screenGenerating {
				return m, tea.Quit
			}
		}
		if msg.String() == "r" && m.screen != screenSplash && m.screen != screenGenerating {
			m.catalogErr = "Refreshing catalog…"
			return m, fetchCatalogCmd(m.apiURL, m.catalogURL, m.catalogPath, m.ignoreEnv)
		}

		switch m.screen {
		case screenSplash:
			return m, nil
		case screenLoading:
			return m.updateLoading(msg)
		case screenProject:
			return m.updateProject(msg)
		case screenPlatforms:
			return m.updatePlatforms(msg)
		case screenSelect:
			return m.updateSelect(msg)
		case screenTemplate:
			return m.updateTemplate(msg)
		case screenPreflight:
			return m.updatePreflight(msg)
		case screenInstall:
			return m.updateInstall(msg)
		case screenGenerating:
			return m, nil
		}
	}
	return m.updateInputs(msg)
}

func (m startModel) View() string {
	switch m.screen {
	case screenSplash:
		return m.viewSplash()
	case screenLoading:
		return m.viewLoading()
	case screenProject:
		return m.viewProject()
	case screenPlatforms:
		return m.viewPlatforms()
	case screenSelect:
		return m.viewSelect()
	case screenTemplate:
		return m.viewTemplate()
	case screenPreflight:
		return m.viewPreflight()
	case screenInstall:
		return m.viewInstall()
	case screenGenerating:
		return m.viewGenerating()
	}
	return ""
}

// --- Views ---

func (m startModel) viewSplash() string {
	frames := []string{
		`  ╔══════════════════════════════════════════════════════════════╗`,
		`  ║                                                              ║`,
		`  ║                          VeloCli                             ║`,
		`  ║                                                              ║`,
		`  ║                Flutter Development Ecosystem                 ║`,
		`  ║                                                              ║`,
		`  ╚══════════════════════════════════════════════════════════════╝`,
	}
	frameIdx := m.splashFrame % len(frames)
	if m.splashFrame < len(frames) {
		frameIdx = m.splashFrame
	}
	var b strings.Builder
	for i := 0; i <= frameIdx; i++ {
		b.WriteString(frames[i])
		if i < frameIdx {
			b.WriteString("\n")
		}
	}
	if m.splashFrame >= len(frames) {
		b.WriteString("\n\n")
		b.WriteString(styleDim.Render("Loading catalog…"))
	}
	return styleBox.Render(b.String())
}

func (m startModel) viewLoading() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("VeloCli") + "\n\n")
	if m.catalogErr != "" {
		b.WriteString(styleError.Render(m.catalogErr) + "\n\n")
		b.WriteString(styleDim.Render("Press r to retry • esc to quit") + "\n")
	} else {
		b.WriteString(styleDim.Render("Loading catalog…") + "\n")
	}
	return styleBox.Render(b.String())
}

func (m startModel) viewProject() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("Project Setup") + "\n")
	b.WriteString(styleKeyHint.Render("enter next • tab switch") + "\n")

	if m.loadErrText != "" {
		b.WriteString(styleError.Render(m.loadErrText) + "\n")
	} else if m.catalogErr != "" {
		b.WriteString(styleWarn.Render(m.catalogErr) + "\n")
	}

	b.WriteString(styleLabel.Render("Project name"))
	b.WriteString("\n" + m.projectName.View() + "\n")

	b.WriteString(styleLabel.Render("Description"))
	b.WriteString("\n" + m.description.View() + "\n")

	b.WriteString(styleLabel.Render("Package / Org"))
	b.WriteString("\n" + m.pkgOrOrg.View() + "\n")

	// IDE Selection
	ideLabel := styleLabel.Render("Open in IDE")
	ideVal := styleDim.Render(m.ideString())
	if m.focusIdx == 3 {
		ideVal = styleCursor.Render("‹ " + m.ideString() + " ›")
		ideVal += styleDim.Render(" (←/→ to change)")
	}
	b.WriteString(ideLabel + "\n" + ideVal + "\n")

	return styleBox.Render(b.String())
}

func (m startModel) viewPlatforms() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("Platforms") + "\n")
	b.WriteString(styleKeyHint.Render("↑/↓ select • space/x toggle • enter next") + "\n")
	for i, p := range platformKeys {
		on := m.platforms[p]
		box := "[ ]"
		if on {
			box = "[x]"
		}

		cursor := "  "
		style := styleDim
		if i == m.platformCursor {
			cursor = styleCursor.Render("› ")
			style = styleSelected // Highlight the focused item
		} else if on {
			style = styleSelected // Highlight selected items even if not focused (optional, but good for visibility)
		}

		// If focused, prefix with cursor. If not, just spaces.
		// Note: We handled cursor above.
		// Construct the line: "› [x] android" or "  [ ] ios"
		
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, style.Render(fmt.Sprintf("%s %s", box, p))))
	}
	return styleBox.Render(b.String())
}

func (m startModel) viewSelect() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("Features") + "\n")
	b.WriteString(styleKeyHint.Render("tab switch col • space/x toggle • enter next") + "\n")

	if m.loadErrText != "" {
		b.WriteString(styleError.Render(m.loadErrText) + "\n")
	}

	// Split View: Categories (Left) | Blocks (Right)
	
	// --- Categories Column ---
	var catList strings.Builder
	catList.WriteString(styleLabel.Render("CATEGORIES") + "\n\n")
	for ci, c := range m.categories {
		activeCat := ci == m.cursor.Category
		title := c.Name
		if title == "" {
			title = c.ID
		}
		
		cursor := "  "
		style := styleDim
		if activeCat {
			if m.focusIdx == 0 { // Categories focused
				cursor = styleCursor.Render("› ")
				style = styleSelected
			} else {
				cursor = "• " // Active but not focused
				style = styleSelected
			}
		}
		catList.WriteString(style.Render(cursor+title) + "\n")
	}
	
	// --- Blocks Column ---
	var blockList strings.Builder
	activeCatID := m.activeCategoryID()
	blocks := m.blocksForCategory(activeCatID)
	
	blockList.WriteString(styleLabel.Render("FEATURES") + "\n\n")
	
	if len(blocks) == 0 {
		blockList.WriteString(styleDim.Render("No features available.") + "\n")
	} else {
		for bi, blk := range blocks {
			isCursor := bi == m.cursor.Block
			box := "[ ]"
			style := styleDim
			if m.selectedBlocks[blk.ID] {
				box = "[x]"
				style = styleSelected
			}
			
			label := blk.Label
			if label == "" {
				label = blk.ID
			}

			row := fmt.Sprintf("%s %s", box, label)
			if isCursor && m.focusIdx == 1 { // Blocks focused
				row = styleCursor.Render("› ") + style.Render(row)
			} else {
				row = "  " + style.Render(row)
			}
			blockList.WriteString(row + "\n")
		}
	}

	// Join Columns
	left := styleBox.Width(30).Render(catList.String())
	right := styleBox.Width(50).Render(blockList.String())
	
	columns := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return lipgloss.JoinVertical(lipgloss.Left, b.String(), columns)
}

func (m startModel) viewTemplate() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("Starting Template") + "\n")
	b.WriteString(styleKeyHint.Render("↑/↓ select • enter next") + "\n")

	for i, t := range m.templates {
		prefix := "  "
		style := styleDim
		if i == m.templateIdx {
			prefix = styleCursor.Render("›") + " "
			style = styleSelected
		}
		label := t.Label
		if label == "" {
			label = t.ID
		}
		b.WriteString(style.Render(prefix + label) + "\n")
	}
	return styleBox.Render(b.String())
}

func (m startModel) viewPreflight() string {
	b := strings.Builder{}
	b.WriteString(styleHeader.Render("Ready to Launch?") + "\n")
	
	// Summary
	b.WriteString(styleLabel.Render("Project: ") + m.projectName.Value() + "\n")
	b.WriteString(styleLabel.Render("Path:    ") + filepath.Join(m.resolveOutputDir(), m.projectName.Value()) + "\n")
	b.WriteString(styleLabel.Render("IDE:     ") + m.ideString() + "\n\n")
	
	b.WriteString(styleKeyHint.Render("enter generate • esc back") + "\n")
	return styleBox.Render(b.String())
}

func (m startModel) viewInstall() string {
	b := strings.Builder{}
	b.WriteString(styleHeader.Render("Missing Dependencies") + "\n\n")
	b.WriteString(styleWarn.Render("Neither 'flutter' nor 'fvm' was found in your PATH.") + "\n")
	b.WriteString("VeloCLI needs Flutter to build your project.\n\n")
	b.WriteString(styleSelected.Render("Press ENTER to attempt automatic installation (latest stable)"))
	b.WriteString("\n" + styleDim.Render("Press ESC to cancel"))
	return styleBox.Render(b.String())
}

func (m startModel) viewGenerating() string {
	return styleHeader.Render("Generating Project...") + "\n" + styleDim.Render("Please wait...")
}

// --- Update Logic ---

func (m startModel) updateProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		m.focusIdx = (m.focusIdx + 1) % 4
		m.syncFocus()
		return m, nil
	case "shift+tab":
		m.focusIdx = (m.focusIdx + 3) % 4 // +3 is equivalent to -1 mod 4
		m.syncFocus()
		return m, nil
	case "ctrl+e":
		m.ide = (m.ide + 1) % 3
		return m, nil
	case "left", "h":
		if m.focusIdx == 3 {
			m.ide--
			if m.ide < 0 { m.ide = 2 }
			return m, nil
		}
	case "right", "l", "space":
		if m.focusIdx == 3 {
			m.ide = (m.ide + 1) % 3
			return m, nil
		}
	case "enter":
		if m.focusIdx < 3 {
			m.focusIdx++
			m.syncFocus()
			return m, nil
		}
		if errText := m.validateProjectInputs(); errText != "" {
			m.loadErrText = errText
			fmt.Print("\a")
			return m, nil
		}
		// Check for duplicate project
		pPath := filepath.Join(m.resolveOutputDir(), m.projectName.Value())
		if _, err := os.Stat(pPath); err == nil {
			m.loadErrText = "Directory already exists: " + pPath
			m.focusIdx = 0 // Reset focus to project name
			m.syncFocus()
			fmt.Print("\a")
			return m, nil
		}
		
		m.loadErrText = ""
		m.screen = screenPlatforms
		return m, nil
	}
	return m.updateInputs(msg)
}

func (m startModel) updatePreflight(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.flutterPath == "" && m.fvmPath == "" {
			m.screen = screenInstall
			return m, nil
		}
		m.screen = screenGenerating
		return m, tea.Batch(
			tea.Quit,
			runGenerationCmd(m, generatorOptions{OutputDir: m.resolveOutputDir(), IDE: m.ide}),
		)
	case "esc", "b":
		m.screen = screenTemplate
		return m, nil
	}
	return m, nil
}

func (m startModel) updateInstall(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.installConfirmed = true
		m.screen = screenGenerating
		return m, tea.Batch(
			tea.Quit,
			runGenerationCmd(m, generatorOptions{OutputDir: m.resolveOutputDir(), IDE: m.ide}),
		)
	case "esc":
		return m, tea.Quit
	}
	return m, nil
}

func (m startModel) resolveOutputDir() string {
	if m.outputDir != "" {
		return m.outputDir
	}
	return getDefaultProjectDir()
}

func getDefaultProjectDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(home, "AndroidStudioProjects")
	default:
		return filepath.Join(home, "StudioProjects")
	}
}

func (m startModel) updatePlatforms(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.screen = screenSelect
		m.clampCursor()
		return m, nil
	case "esc":
		m.screen = screenProject
		m.syncFocus()
		return m, nil
	case "up", "k":
		if m.platformCursor > 0 {
			m.platformCursor--
		}
	case "down", "j":
		if m.platformCursor < len(platformKeys)-1 {
			m.platformCursor++
		}
	case "space", "x", " ":
		if m.platformCursor >= 0 && m.platformCursor < len(platformKeys) {
			p := platformKeys[m.platformCursor]
			m.togglePlatform(p)
		}
	case "a": m.togglePlatform("android")
	case "i": m.togglePlatform("ios")
	case "w": m.togglePlatform("web")
	}
	return m, nil
}

func (m startModel) updateSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			if m.focusIdx == 0 {
				m.focusIdx = 1
				m.cursor.Block = 0
			} else {
				m.focusIdx = 0
			}
		case "right", "l":
			if m.focusIdx == 0 {
				m.focusIdx = 1
				m.cursor.Block = 0
			}
		case "left", "h":
			if m.focusIdx == 1 {
				m.focusIdx = 0
			}
		case "up", "k":
			if m.focusIdx == 0 {
				if m.cursor.Category > 0 {
					m.cursor.Category--
					m.cursor.Block = 0
				}
			} else {
				if m.cursor.Block > 0 {
					m.cursor.Block--
				}
			}
		case "down", "j":
			if m.focusIdx == 0 {
				if m.cursor.Category < len(m.categories)-1 {
					m.cursor.Category++
					m.cursor.Block = 0
				}
			} else {
				blocks := m.blocksForCategory(m.activeCategoryID())
				if m.cursor.Block < len(blocks)-1 {
					m.cursor.Block++
				}
			}
		case "space", "x", " ":
			if m.focusIdx == 1 {
				m.toggleBlock()
				fmt.Print("\a") // Sound feedback
			}
		case "enter":
			if m.focusIdx == 0 {
				m.focusIdx = 1
				m.cursor.Block = 0
				return m, nil
			}
			m.screen = screenTemplate
		case "esc":
			m.screen = screenPlatforms
		}
	}
	return m, nil
}

func (m startModel) updateTemplate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.templateIdx > 0 {
			m.templateIdx--
		}
	case "down", "j":
		if m.templateIdx < len(m.templates)-1 {
			m.templateIdx++
		}
	case "enter":
		m.screen = screenPreflight
	case "esc":
		m.screen = screenSelect
	}
	return m, nil
}

func (m startModel) updateInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focusIdx {
	case 0:
		m.projectName, cmd = m.projectName.Update(msg)
	case 1:
		m.description, cmd = m.description.Update(msg)
	case 2:
		m.pkgOrOrg, cmd = m.pkgOrOrg.Update(msg)
	}
	return m, cmd
}

func (m startModel) updateLoading(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		return m, tea.Quit
	}
	return m, nil
}

func (m startModel) togglePlatform(p string) {
	if _, ok := m.platforms[p]; ok {
		m.platforms[p] = !m.platforms[p]
	}
}

func (m startModel) toggleBlock() {
	activeCatID := m.activeCategoryID()
	blocks := m.blocksForCategory(activeCatID)
	if m.cursor.Block < 0 || m.cursor.Block >= len(blocks) {
		return
	}
	blk := blocks[m.cursor.Block]
	
	// Check category selection mode
	var cat Category
	for _, c := range m.categories {
		if c.ID == activeCatID {
			cat = c
			break
		}
	}

	if m.selectedBlocks[blk.ID] {
		// Deselect
		delete(m.selectedBlocks, blk.ID)
		return
	}

	// Select
	if cat.SelectionMode == SelectionModeSingle {
		// Deselect other blocks in this category
		for _, b := range blocks {
			delete(m.selectedBlocks, b.ID)
		}
	}
	m.selectedBlocks[blk.ID] = true
}

func (m startModel) activeCategoryID() string {
	if len(m.categories) > m.cursor.Category {
		return m.categories[m.cursor.Category].ID
	}
	return ""
}

func (m startModel) blocksForCategory(id string) []Block {
	var out []Block
	for _, b := range m.blocks {
		if b.CategoryID == id {
			out = append(out, b)
		}
	}
	return out
}

func (m startModel) ideString() string {
	switch m.ide {
	case ideVSCode: return "VS Code"
	case ideAndroidStudio: return "Android Studio"
	default: return "None"
	}
}

func (m startModel) validateProjectInputs() string {
	if m.projectName.Value() == "" { return "Project name required" }
	if m.pkgOrOrg.Value() == "" { return "Package/Org required" }
	return ""
}

func (m *startModel) syncFocus() {
	m.projectName.Blur()
	m.description.Blur()
	m.pkgOrOrg.Blur()
	switch m.focusIdx {
	case 0: m.projectName.Focus()
	case 1: m.description.Focus()
	case 2: m.pkgOrOrg.Focus()
	}
}

func (m *startModel) purgeMissingSelections() {}
func (m *startModel) clampCursor() {}

type flutterCheckMsg struct {
	flutterPath string
	fvmPath     string
}

func checkFlutterCmd() tea.Cmd {
	return func() tea.Msg {
		f, _ := exec.LookPath("flutter")
		fv, _ := exec.LookPath("fvm")
		return flutterCheckMsg{flutterPath: f, fvmPath: fv}
	}
}

func fetchCatalogCmd(apiURL, catalogURL, catalogPath string, ignoreEnv bool) tea.Cmd {
	return func() tea.Msg {
		cat, err := FetchCatalog(apiURL, catalogURL, catalogPath, ignoreEnv)
		if err != nil {
			return catalogLoadFailedMsg{err: err}
		}
		return catalogLoadedMsg{catalog: *cat}
	}
}

func startCatalogStreamCmd(stream *catalogStream, apiURL string) tea.Cmd {
	return func() tea.Msg {
		stream.startIfNeeded(apiURL)
		return nil
	}
}

func waitCatalogStreamCmd(stream *catalogStream) tea.Cmd {
	return func() tea.Msg {
		ev := <-stream.events
		return catalogStreamMsg{ev: ev}
	}
}

func splashTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return splashTickMsg{}
	})
}

func runGenerationCmd(m startModel, opts generatorOptions) tea.Cmd {
	return func() tea.Msg {
		err := runGeneration(m, opts)
		if err != nil {
			fmt.Println("\nError:", err)
		}
		return tea.Quit()
	}
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" { return v }
	return def
}

type catalogLoadedMsg struct {
	catalog Catalog
	updateAvailable bool
	latestVersion string
}

type catalogLoadFailedMsg struct { err error }

type catalogStreamMsg struct { ev catalogStreamEvent }

type splashTickMsg struct{}

func FetchCatalog(apiURL, catalogURL, catalogPath string, ignoreEnv bool) (*Catalog, error) {
	// If direct URL or Path is provided (ignoring env or not), use it.
	// Logic simplified:
	url := apiURL + "/api/v1/catalog"
	if catalogURL != "" {
		url = catalogURL
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	// Send client version for server-side enforcement
	req.Header.Set("X-VeloCLI-Version", Version)
	
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
		return nil, fmt.Errorf("catalog fetch failed: %s (%s)", res.Status, strings.TrimSpace(string(body)))
	}
	
	var cat Catalog
	if err := json.NewDecoder(res.Body).Decode(&cat); err != nil {
		return nil, err
	}
	return &cat, nil
}

func runGeneration(m startModel, opts generatorOptions) error {
	projectPath := filepath.Join(opts.OutputDir, m.projectName.Value())
	fmt.Printf("\nCreating project at %s...\n", projectPath)

	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return err
	}

	// Install Flutter if requested and missing
	if m.installConfirmed && m.flutterPath == "" && m.fvmPath == "" {
		fmt.Println("Attempting to install Flutter (stable)...")
		var installCmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			// Try brew
			if _, err := exec.LookPath("brew"); err == nil {
				installCmd = exec.Command("brew", "install", "--cask", "flutter")
			}
		}
		if installCmd == nil {
			// Fallback to warning
			fmt.Println("Automatic installation not supported on this OS. Please install Flutter manually.")
		} else {
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
			if err := installCmd.Run(); err != nil {
				fmt.Printf("Installation failed: %v\n", err)
			} else {
				// Re-check path
				m.flutterPath, _ = exec.LookPath("flutter")
			}
		}
	}

	// Determine create command
	var cmd *exec.Cmd
	if m.fvmPath != "" {
		fmt.Println("Using FVM...")
		cmd = exec.Command(m.fvmPath, "flutter", "create", ".", "--org", m.pkgOrOrg.Value(), "--project-name", m.projectName.Value())
	} else if m.flutterPath != "" {
		fmt.Println("Using Flutter...")
		cmd = exec.Command(m.flutterPath, "create", ".", "--org", m.pkgOrOrg.Value(), "--project-name", m.projectName.Value())
	} else {
		return fmt.Errorf("flutter/fvm not found. Cannot create project")
	}

	cmd.Dir = projectPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("flutter create failed: %w", err)
	}

	var tpl MainTemplate
	if len(m.templates) > 0 && m.templateIdx >= 0 && m.templateIdx < len(m.templates) {
		tpl = m.templates[m.templateIdx]
	}

	if tpl.ID != "" {
		fmt.Println("Applying template...")
		if err := applyMainTemplate(m.apiURL, tpl, projectPath); err != nil {
			return fmt.Errorf("failed to apply template: %w", err)
		}
	}

	var selectedIDs []string
	for id := range m.selectedBlocks {
		selectedIDs = append(selectedIDs, id)
	}
	fmt.Println("Applying selected features...")
	if err := applySelectedBlocks(m.apiURL, m.blocks, selectedIDs, projectPath); err != nil {
		return fmt.Errorf("failed to apply blocks: %w", err)
	}

	// Pub Get
	fmt.Println("Running pub get...")
	var pubCmd *exec.Cmd
	if m.fvmPath != "" {
		pubCmd = exec.Command(m.fvmPath, "flutter", "pub", "get")
	} else {
		pubCmd = exec.Command(m.flutterPath, "pub", "get")
	}
	pubCmd.Dir = projectPath
	pubCmd.Stdout = os.Stdout
	pubCmd.Stderr = os.Stderr
	_ = pubCmd.Run() // Ignore error, non-fatal

	// Open IDE
	if opts.IDE != ideNone {
		fmt.Println("Opening IDE...")
		var ideCmd *exec.Cmd
		switch opts.IDE {
		case ideVSCode:
			ideCmd = exec.Command("code", ".")
		case ideAndroidStudio:
			if runtime.GOOS == "darwin" {
				ideCmd = exec.Command("open", "-a", "Android Studio", ".")
			} else if runtime.GOOS == "windows" {
				ideCmd = exec.Command("studio64.exe", ".") // Best guess for Windows
			} else {
				ideCmd = exec.Command("studio", ".") // Linux
			}
		}
		if ideCmd != nil {
			ideCmd.Dir = projectPath
			_ = ideCmd.Start()
		}
	}

	fmt.Println("\nDone! 🚀")
	return nil
}
