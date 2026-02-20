package commands

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func NewCreateCmd() *cobra.Command {
	var apiURL string
	var catalogURL string
	var catalogPath string
	var outputDir string
	var ide string

	var projectName string
	var description string
	var org string
	var platforms string
	var blocks string
	var templateID string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Generate a project non-interactively (for IDE plugins/CI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if apiURL == "" {
				apiURL = envDefault("VELOCLI_API_URL", "http://localhost:9999")
			}

			ignoreEnv := cmd.Flags().Changed("api-url") && !cmd.Flags().Changed("catalog-url") && !cmd.Flags().Changed("catalog-path")
			cat, err := FetchCatalog(apiURL, catalogURL, catalogPath, ignoreEnv)
			if err != nil {
				return err
			}

			sm := NewStartModelPlain(apiURL, catalogURL, catalogPath, ignoreEnv)
			sm.categories = cat.Categories
			sm.blocks = cat.Blocks
			sm.templates = cat.MainTemplates

			projectName = strings.TrimSpace(projectName)
			if projectName != "" {
				sm.projectName.SetValue(projectName)
			}
			description = strings.TrimSpace(description)
			if description != "" {
				sm.description.SetValue(description)
			}
			org = strings.TrimSpace(org)
			if org != "" {
				sm.pkgOrOrg.SetValue(org)
				sm.pkgTouched = true
			}

			sm.selectedBlocks = map[string]bool{}
			for _, id := range splitCSV(blocks) {
				if id == "" {
					continue
				}
				sm.selectedBlocks[id] = true
			}

			sm.platforms = map[string]bool{
				"android": false,
				"ios":     false,
				"web":     false,
				"macos":   false,
				"windows": false,
				"linux":   false,
			}
			for _, p := range splitCSV(platforms) {
				if p == "" {
					continue
				}
				if _, ok := sm.platforms[p]; ok {
					sm.platforms[p] = true
				}
			}

			if templateID != "" && len(sm.templates) > 0 {
				for i := range sm.templates {
					if sm.templates[i].ID == templateID {
						sm.templateIdx = i
						break
					}
				}
			}

			opts := generatorOptions{
				OutputDir: strings.TrimSpace(outputDir),
				IDE:       ParseIDE(ide),
			}
			return runGeneration(sm, opts)
		},
	}

	cmd.Flags().StringVar(&apiURL, "api-url", "", "Backend base URL")
	cmd.Flags().StringVar(&catalogURL, "catalog-url", "", "Full catalog URL (overrides api-url)")
	cmd.Flags().StringVar(&catalogPath, "catalog-path", "", "Catalog path (appended to api-url)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory where projects are created")
	cmd.Flags().StringVar(&ide, "ide", "none", "Open IDE after generation: none|vscode|android-studio")

	cmd.Flags().StringVar(&projectName, "project-name", "my_app", "Flutter project name (lower_snake_case)")
	cmd.Flags().StringVar(&description, "description", "A Flutter app built with VeloCLI", "Project description")
	cmd.Flags().StringVar(&org, "org", "com.company", "Organization/package base (com.company or com.company.my_app)")
	cmd.Flags().StringVar(&platforms, "platforms", "android,ios", "Comma-separated platforms (android,ios,web,macos,windows,linux)")
	cmd.Flags().StringVar(&blocks, "blocks", "", "Comma-separated block IDs to apply")
	cmd.Flags().StringVar(&templateID, "template", "", "main.dart template ID")

	return cmd
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	slices.Sort(out)
	out = slices.Compact(out)
	return out
}

func fetchCatalog(apiURL string, catalogURL string, catalogPath string, ignoreEnv bool) (Catalog, error) {
	cat, err := FetchCatalog(apiURL, catalogURL, catalogPath, ignoreEnv)
	if err != nil {
		return Catalog{}, err
	}
	return *cat, nil
}
