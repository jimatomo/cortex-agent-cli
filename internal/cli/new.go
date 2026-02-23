package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"coragent/internal/agent"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var avatarOptions = []string{
	"SparklesAgentIcon",
	"ShieldBoltAgentIcon",
	"GlobeAgentIcon",
	"BookAgentIcon",
	"PencilAgentIcon",
	"BlocksAgentIcon",
	"BrochureAgentIcon",
	"ChartAgentIcon",
	"CirclesAgentIcon",
	"ComputeAgentIcon",
	"DocumentAgentIcon",
	"EducationAgentIcon",
	"IdeaAgentIcon",
	"PhoneAgentIcon",
	"PowerAgentIcon",
	"QuestionAgentIcon",
	"RobotAgentIcon",
	"VerifiedAgentIcon",
	"WandAgentIcon",
	"WorkAgentIcon",
}

var colorOptions = []struct {
	label string
	value string
}{
	{"Blue", "var(--chartDim_1-x11ij0mo)"},
	{"Pink", "var(--chartDim_8-x1mzf9u0)"},
	{"Green", "var(--chartDim_3-x11sbcwy)"},
	{"Red", "var(--chartDim_4-x1kcru7n)"},
	{"Purple", "var(--chartDim_5-x5ky746)"},
	{"Orange", "var(--chartDim_6-x12aliq8)"},
}

func newNewCmd(_ *RootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "new",
		Short: "Interactively create a new agent YAML spec",
		Long: `Interactively walk through all agent configuration fields and write a new
agent YAML file. No flags — the wizard prompts for every value.

Example:
  coragent new`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew()
		},
	}
}

func runNew() error {
	fmt.Println("Agent Spec Wizard")
	fmt.Println("=================")
	fmt.Println()

	// --- Mode selection ---
	fmt.Println("Mode:")
	fmt.Println("  1) Full    - configure all fields")
	fmt.Println("  2) Minimum - name, tools only")
	modeChoice, err := promptWithDefault("Choose [1/2]", "1")
	if err != nil {
		return err
	}
	minimumMode := strings.TrimSpace(modeChoice) == "2"
	fmt.Println()

	// --- Output file ---
	outFile, err := promptWithDefault("Output file", "agent.yml")
	if err != nil {
		return err
	}
	if outFile == "" {
		outFile = "agent.yml"
	}

	if _, statErr := os.Stat(outFile); statErr == nil {
		ans, err := promptWithDefault(fmt.Sprintf("File %q already exists. Overwrite? [y/N]", outFile), "N")
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// --- Basic Info ---
	fmt.Println()
	fmt.Println("--- Basic Info ---")

	var agentName string
	if minimumMode {
		// Derive agent name from output filename (strip directory and extension)
		base := outFile
		if idx := strings.LastIndexByte(base, '/'); idx >= 0 {
			base = base[idx+1:]
		}
		if dot := strings.LastIndex(base, "."); dot > 0 {
			base = base[:dot]
		}
		agentName = base
		fmt.Printf("  Agent name: %s\n", agentName)
	} else {
		agentName, err = promptWithDefault("Agent name", "")
		if err != nil {
			return err
		}
		if agentName == "" {
			return UserErr(fmt.Errorf("agent name is required"))
		}
	}

	var comment string
	if !minimumMode {
		comment, err = promptWithDefault("Comment (optional)", "")
		if err != nil {
			return err
		}
	}

	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: comment,
	}

	if minimumMode {
		// --- Display name (minimum mode only) ---
		displayName, err := promptWithDefault("Display name (optional)", "")
		if err != nil {
			return err
		}
		if displayName != "" {
			spec.Profile = &agent.Profile{DisplayName: displayName}
		}
	}

	if !minimumMode {
		// --- Profile (optional) ---
		fmt.Println()
		fmt.Println("--- Profile (optional) ---")

		configProfile, err := promptWithDefault("Configure profile? [y/N]", "N")
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(configProfile)) == "y" {
			profile := &agent.Profile{}

			displayName, err := promptWithDefault("Display name", "")
			if err != nil {
				return err
			}
			profile.DisplayName = displayName

			// Avatar selection
			fmt.Println()
			for i, av := range avatarOptions {
				label := strings.TrimSuffix(av, "AgentIcon")
				col := (i % 3) + 1
				if col == 3 || i == len(avatarOptions)-1 {
					fmt.Printf("  %2d) %s\n", i+1, label)
				} else {
					fmt.Printf("  %2d) %-14s", i+1, label)
				}
			}
			avatarChoice, err := promptWithDefault("Choose [1-20] (optional, Enter to skip)", "")
			if err != nil {
				return err
			}
			if avatarChoice != "" {
				if idx, convErr := strconv.Atoi(strings.TrimSpace(avatarChoice)); convErr == nil && idx >= 1 && idx <= len(avatarOptions) {
					profile.Avatar = avatarOptions[idx-1]
				}
			}

			// Color selection
			fmt.Println()
			var colorLabels []string
			for i, co := range colorOptions {
				colorLabels = append(colorLabels, fmt.Sprintf("%d) %s", i+1, co.label))
			}
			fmt.Printf("  %s\n", strings.Join(colorLabels, "   "))
			colorChoice, err := promptWithDefault("Choose [1-6] (optional, Enter to skip)", "")
			if err != nil {
				return err
			}
			if colorChoice != "" {
				if idx, convErr := strconv.Atoi(strings.TrimSpace(colorChoice)); convErr == nil && idx >= 1 && idx <= len(colorOptions) {
					profile.Color = colorOptions[idx-1].value
				}
			}

			if profile.DisplayName != "" || profile.Avatar != "" || profile.Color != "" {
				spec.Profile = profile
			}
		}

		// --- Model ---
		fmt.Println()
		fmt.Println("--- Model ---")

		orchModel, err := promptWithDefault("Orchestration model", "auto")
		if err != nil {
			return err
		}
		if orchModel != "" && orchModel != "auto" {
			spec.Models = &agent.Models{Orchestration: orchModel}
		}

		// --- Instructions ---
		fmt.Println()
		fmt.Println("--- Instructions ---")

		systemPrompt, err := readMultilinePrompt("System prompt (enter blank line to finish, optional)")
		if err != nil {
			return err
		}

		orchPrompt, err := readMultilinePrompt("Orchestration prompt (optional, blank line to finish)")
		if err != nil {
			return err
		}

		responsePrompt, err := readMultilinePrompt("Response prompt (optional, blank line to finish)")
		if err != nil {
			return err
		}

		var sampleQuestions []agent.SampleQuestion
		addQuestionsAns, err := promptWithDefault("Add sample questions? [y/N]", "N")
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(addQuestionsAns)) == "y" {
			for {
				q, err := promptWithDefault("Question", "")
				if err != nil {
					return err
				}
				if q != "" {
					sampleQuestions = append(sampleQuestions, agent.SampleQuestion{Question: q})
				}
				moreAns, err := promptWithDefault("Add another? [y/N]", "N")
				if err != nil {
					return err
				}
				if strings.ToLower(strings.TrimSpace(moreAns)) != "y" {
					break
				}
			}
		}

		if systemPrompt != "" || orchPrompt != "" || responsePrompt != "" || len(sampleQuestions) > 0 {
			instr := &agent.Instructions{}
			if systemPrompt != "" {
				instr.System = systemPrompt
			}
			if orchPrompt != "" {
				instr.Orchestration = orchPrompt
			}
			if responsePrompt != "" {
				instr.Response = responsePrompt
			}
			if len(sampleQuestions) > 0 {
				instr.SampleQuestions = sampleQuestions
			}
			spec.Instructions = instr
		}

		// --- Orchestration Budget ---
		fmt.Println()
		fmt.Println("--- Orchestration Budget ---")

		budgetSecsStr, err := promptWithDefault("Budget seconds (e.g. 300, optional)", "")
		if err != nil {
			return err
		}
		budgetTokensStr, err := promptWithDefault("Budget tokens (e.g. 16000, optional)", "")
		if err != nil {
			return err
		}

		var budgetCfg *agent.BudgetConfig
		budgetSecs, secsOK := strconv.Atoi(strings.TrimSpace(budgetSecsStr))
		budgetTokens, tokensOK := strconv.Atoi(strings.TrimSpace(budgetTokensStr))
		if secsOK == nil || tokensOK == nil {
			budgetCfg = &agent.BudgetConfig{}
			if secsOK == nil {
				budgetCfg.Seconds = budgetSecs
			}
			if tokensOK == nil {
				budgetCfg.Tokens = budgetTokens
			}
		}
		if budgetCfg != nil {
			spec.Orchestration = &agent.Orchestration{Budget: budgetCfg}
		}
	}

	// --- Tools ---
	fmt.Println()
	fmt.Println("--- Tools ---")

	toolResources := agent.ToolResources{}
	for {
		fmt.Println()
		fmt.Println("  Tool type:")
		fmt.Println("    1) cortex_analyst_text_to_sql")
		fmt.Println("    2) cortex_search")
		fmt.Println("    3) Skip / add later")
		toolTypeChoice, err := promptWithDefault("Choose [1/2/3]", "3")
		if err != nil {
			return err
		}
		if strings.TrimSpace(toolTypeChoice) == "3" || strings.TrimSpace(toolTypeChoice) == "" {
			break
		}

		toolName, err := promptWithDefault("Tool name", "")
		if err != nil {
			return err
		}
		if toolName == "" {
			return UserErr(fmt.Errorf("tool name is required"))
		}

		toolDesc, err := promptWithDefault("Tool description", "")
		if err != nil {
			return err
		}

		toolSpec := map[string]any{
			"name": toolName,
		}
		if toolDesc != "" {
			toolSpec["description"] = toolDesc
		}

		resource := map[string]any{}

		switch strings.TrimSpace(toolTypeChoice) {
		case "2":
			toolSpec["type"] = "cortex_search"

			searchService, err := promptWithDefault("Search service (e.g., DB.SCHEMA.SERVICE_NAME)", "")
			if err != nil {
				return err
			}
			if searchService != "" {
				resource["search_service"] = searchService
			}

			idCol, err := promptWithDefault("ID column (optional)", "")
			if err != nil {
				return err
			}
			if idCol != "" {
				resource["id_column"] = idCol
			}

			titleCol, err := promptWithDefault("Title column (optional)", "")
			if err != nil {
				return err
			}
			if titleCol != "" {
				resource["title_column"] = titleCol
			}

			maxResultsStr, err := promptWithDefault("Max results", "4")
			if err != nil {
				return err
			}
			maxResults := 4
			if v, convErr := strconv.Atoi(strings.TrimSpace(maxResultsStr)); convErr == nil {
				maxResults = v
			}
			if maxResults != 4 {
				resource["max_results"] = maxResults
			}

		default:
			toolSpec["type"] = "cortex_analyst_text_to_sql"

			semanticView, err := promptWithDefault("Semantic view (e.g., DB.SCHEMA.VIEW_NAME)", "")
			if err != nil {
				return err
			}
			if semanticView != "" {
				resource["semantic_view"] = semanticView
			}

			warehouse, err := promptWithDefault("Warehouse (optional)", "")
			if err != nil {
				return err
			}

			queryTimeoutStr, err := promptWithDefault("Query timeout seconds (e.g. 60, optional)", "")
			if err != nil {
				return err
			}

			execEnv := map[string]any{
				"type": "warehouse",
			}
			if warehouse != "" {
				execEnv["warehouse"] = warehouse
			}
			if v, convErr := strconv.Atoi(strings.TrimSpace(queryTimeoutStr)); convErr == nil {
				execEnv["query_timeout"] = v
			}
			resource["execution_environment"] = execEnv
		}

		spec.Tools = append(spec.Tools, agent.Tool{ToolSpec: toolSpec})
		if len(resource) > 0 {
			toolResources[toolName] = resource
		}
		fmt.Println()
	}

	if len(toolResources) > 0 {
		spec.ToolResources = toolResources
	}

	// --- Write ---
	var doc yaml.Node
	if err := doc.Encode(spec); err != nil {
		return fmt.Errorf("marshal YAML: %w", err)
	}
	setLiteralStyleForMultiline(&doc)
	reorderExportKeys(&doc)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("marshal YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("flush YAML encoder: %w", err)
	}

	fmt.Printf("\nWriting %s...\n", outFile)
	if err := os.WriteFile(outFile, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %q: %w", outFile, err)
	}
	fmt.Printf("Done! Run 'coragent validate %s' to verify.\n", outFile)
	return nil
}

// readMultilinePrompt reads lines of input until the user enters an empty line.
// Returns the lines joined by "\n", or "" if no lines were entered.
func readMultilinePrompt(label string) (string, error) {
	fmt.Printf("  %s:\n", label)
	var lines []string
	for {
		line, err := readLine("  > ")
		if err != nil {
			if errors.Is(err, errInterrupted) {
				return "", UserErr(fmt.Errorf("cancelled"))
			}
			return "", err
		}
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}
