package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type Environment map[string]string

type EnvConfig struct {
	Environments map[string]Environment `yaml:"environments"`
}

func Execute() error {
	rootCmd := NewRootCmd()
	rootCmd.AddCommand(NewGenerateCmd())
	rootCmd.AddCommand(NewCompletionCmd(rootCmd))
	return rootCmd.Execute()
}

func NewRootCmd() *cobra.Command {
	var envName string

	cmd := &cobra.Command{
		Use:   "curly [collection-dir]",
		Short: "Fuzzy-find an endpoint (.curl) and open in $EDITOR, then run on save/exit",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return launchCollection(dir, envName)
		},
	}

	cmd.Flags().StringVarP(&envName, "env", "e", "", "Environment name to use from envs.yml")

	return cmd
}

func launchCollection(dir string, envName string) error {
	var envVars Environment
	if envName != "" {
		envsFile := filepath.Join(dir, "envs.yml")
		config, err := loadEnvConfig(envsFile)
		if err != nil {
			return fmt.Errorf("failed to load envs.yml: %w", err)
		}

		env, ok := config.Environments[envName]
		if !ok {
			return fmt.Errorf("environment '%s' not found in envs.yml", envName)
		}
		envVars = env
	}

	matches := []string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".curl") {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return errors.New("no .curl files found in directory")
	}

	selected, err := fzfSelect(matches)
	if err != nil {
		return err
	}
	if selected == "" {
		return nil
	}

	content, err := ioutil.ReadFile(selected)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)
	if envName != "" {
		contentStr = applyEnvironmentVars(contentStr, envVars)
		tmpFile := selected + ".tmp"
		if err := ioutil.WriteFile(tmpFile, []byte(contentStr), 0644); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		selected = tmpFile
		defer os.Remove(tmpFile)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nvim"
	}

	editCmd := exec.Command(editor, selected)
	editCmd.Stdin = os.Stdin
	editCmd.Stdout = os.Stdout
	editCmd.Stderr = os.Stderr
	if err := editCmd.Run(); err != nil {
		return fmt.Errorf("editor failed: %w", err)
	}

	content, err = ioutil.ReadFile(selected)
	if err != nil {
		return fmt.Errorf("failed to read file after editing: %w", err)
	}

	cmdText := extractShellCommand(string(content))
	if cmdText == "" {
		return errors.New("no curl command found in file")
	}

	execCmd := exec.Command("sh", "-c", cmdText)
	execCmd.Stdin = os.Stdin
	out, err := execCmd.CombinedOutput()
	fmt.Printf("%s\n", string(out))
	if err != nil {
		fmt.Fprintf(os.Stderr, "command exited with error: %v\n", err)
	}

	return nil
}

func loadEnvConfig(filename string) (*EnvConfig, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config EnvConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func applyEnvironmentVars(content string, envVars Environment) string {
	lines := strings.Split(content, "\n")
	result := []string{}

	inVarSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "# Variables" {
			inVarSection = true
			result = append(result, line)
			continue
		}

		if inVarSection && (trimmed == "" || strings.HasPrefix(trimmed, "curl")) {
			inVarSection = false
		}

		if inVarSection && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				varName := strings.TrimSpace(parts[0])
				if val, ok := envVars[varName]; ok {
					result = append(result, fmt.Sprintf("%s=\"%s\"", varName, val))
					continue
				}
			}
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func fzfSelect(items []string) (string, error) {
	fzfPath, err := exec.LookPath("fzf")
	if err != nil {
		if len(items) == 1 {
			return items[0], nil
		}
		fmt.Println("fzf not found. Please choose an item by number:")
		for i, it := range items {
			fmt.Printf("[%d] %s\n", i+1, it)
		}
		fmt.Print("Select number: ")
		var idx int
		_, err := fmt.Scanf("%d", &idx)
		if err != nil || idx < 1 || idx > len(items) {
			return "", errors.New("invalid selection")
		}
		return items[idx-1], nil
	}

	input := strings.Join(items, "\n")
	fzfCmd := exec.Command(fzfPath, "--prompt", "Select endpoint: ")
	fzfCmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	fzfCmd.Stdout = &out
	fzfCmd.Stderr = os.Stderr
	if err := fzfCmd.Run(); err != nil {
		return "", fmt.Errorf("fzf failed: %w", err)
	}
	res := strings.TrimSpace(out.String())
	return res, nil
}

func extractShellCommand(content string) string {
	lines := strings.Split(content, "\n")
	result := []string{}
	foundStart := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !foundStart {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			foundStart = true
		}

		if foundStart {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

func NewCompletionCmd(rootCmd *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion script",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := args[0]
			switch shell {
			case "bash":
				return rootCmd.GenBashCompletion(os.Stdout)
			case "zsh":
				return rootCmd.GenZshCompletion(os.Stdout)
			case "fish":
				return rootCmd.GenFishCompletion(os.Stdout, true)
			default:
				return fmt.Errorf("unsupported shell: %s", shell)
			}
		},
	}
	return cmd
}
