package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type Environment map[string]string

type EnvConfig struct {
	Environments map[string]Environment `yaml:"environments"`
}

type ExecutionStats struct {
	Total     int
	Success   int32
	Failed    int32
	StartTime time.Time
	EndTime   time.Time
	Errors    []string
	errorsMux sync.Mutex
}

func (s *ExecutionStats) RecordSuccess() {
	atomic.AddInt32(&s.Success, 1)
}

func (s *ExecutionStats) RecordFailure(err error) {
	atomic.AddInt32(&s.Failed, 1)
	s.errorsMux.Lock()
	s.Errors = append(s.Errors, err.Error())
	s.errorsMux.Unlock()
}

func (s *ExecutionStats) Print() {
	duration := s.EndTime.Sub(s.StartTime)

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Summary:\n")
	fmt.Fprintf(os.Stderr, "  Total:      %d\n", s.Total)
	fmt.Fprintf(os.Stderr, "  Success:    %d\n", s.Success)
	fmt.Fprintf(os.Stderr, "  Failed:     %d\n", s.Failed)
	fmt.Fprintf(os.Stderr, "  Duration:   %s\n", duration.Round(time.Millisecond))

	if s.Total > 0 {
		avgTime := duration / time.Duration(s.Total)
		fmt.Fprintf(os.Stderr, "  Avg time:   %s\n", avgTime.Round(time.Millisecond))

		if duration.Seconds() > 0 {
			throughput := float64(s.Total) / duration.Seconds()
			fmt.Fprintf(os.Stderr, "  Throughput: %.2f req/s\n", throughput)
		}
	}

	if len(s.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\nErrors:\n")
		errorCounts := make(map[string]int)
		for _, err := range s.Errors {
			errorCounts[err]++
		}
		for errMsg, count := range errorCounts {
			if count > 1 {
				fmt.Fprintf(os.Stderr, "  [%dx] %s\n", count, errMsg)
			} else {
				fmt.Fprintf(os.Stderr, "  %s\n", errMsg)
			}
		}
	}
}

var outputMutex sync.Mutex

func Execute() error {
	rootCmd := NewRootCmd()
	rootCmd.AddCommand(NewGenerateCmd())
	rootCmd.AddCommand(NewCompletionCmd(rootCmd))
	return rootCmd.Execute()
}

func NewRootCmd() *cobra.Command {
	var envName string
	var filePath string
	var times int
	var parallel int
	var delay int
	var verbose bool
	var insecure bool

	cmd := &cobra.Command{
		Use:   "curly [collection-dir]",
		Short: "Fuzzy-find an endpoint (.curl) and open in $EDITOR, then run on save/exit",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}

			if times < 1 {
				return fmt.Errorf("times must be at least 1, got %d", times)
			}
			if parallel < 1 {
				return fmt.Errorf("parallel must be at least 1, got %d", parallel)
			}
			if delay < 0 {
				return fmt.Errorf("delay cannot be negative, got %d", delay)
			}

			if parallel > times {
				parallel = times
			}

			cmdText, err := func() (string, error) {
				if filePath != "" {
					return runFile(filePath, dir, envName, insecure)
				}
				return launchCollection(dir, envName, insecure)
			}()
			if err != nil {
				return err
			}
			return execCmd(cmdText, times, parallel, delay, verbose)
		},
	}

	cmd.Flags().StringVarP(&envName, "env", "e", "", "Environment name to use from envs.yml")
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Run a specific .curl file without opening editor")
	cmd.Flags().IntVarP(&times, "times", "n", 1, "Number of times to execute the request")
	cmd.Flags().IntVarP(&parallel, "parallel", "p", 1, "Number of concurrent executions per batch")
	cmd.Flags().IntVar(&delay, "delay", 0, "Delay between batches in seconds")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show progress and detailed output")
	cmd.Flags().BoolVarP(&insecure, "insecure", "k", false, "Skip SSL certificate verification (adds -k to ALL curls in the file)")

	return cmd
}

func launchCollection(dir string, envName string, insecure bool) (string, error) {
	var envVars Environment
	if envName != "" {
		var err error
		envVars, err = loadEnvironmentVariables(envName, dir)
		if err != nil {
			return "", err
		}
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
		return "", err
	}
	if len(matches) == 0 {
		return "", errors.New("no .curl files found in directory")
	}

	selected, err := fzfSelect(matches)
	if err != nil {
		return "", err
	}
	if selected == "" {
		return "", nil
	}

	content, err := os.ReadFile(selected)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)
	if insecure {
		contentStr = strings.ReplaceAll(contentStr, "curl ", "curl -k ")
	}
	if envName != "" {
		contentStr = applyEnvironmentVars(contentStr, envVars)
	}
	tmpFile := selected + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(contentStr), 0644); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	selected = tmpFile
	defer os.Remove(tmpFile)

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	editCmd := exec.Command(editor, selected)
	editCmd.Stdin = os.Stdin
	editCmd.Stdout = os.Stdout
	editCmd.Stderr = os.Stderr
	if err := editCmd.Run(); err != nil {
		return "", fmt.Errorf("editor failed: %w", err)
	}

	content, err = os.ReadFile(selected)
	if err != nil {
		return "", fmt.Errorf("failed to read file after editing: %w", err)
	}

	cmdText := extractShellCommand(string(content))
	if cmdText == "" {
		return "", errors.New("no curl command found in file")
	}

	return cmdText, nil
}

func loadEnvironmentVariables(envName string, dir string) (Environment, error) {
	envsFile := filepath.Join(dir, "envs.yml")
	config, err := loadEnvConfig(envsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load envs.yml: %w", err)
	}

	env, ok := config.Environments[envName]
	if !ok {
		return nil, fmt.Errorf("environment '%s' not found in envs.yml", envName)
	}
	return env, nil
}

func execCmd(cmdText string, times int, parallel int, delay int, verbose bool) error {
	if parallel < 1 {
		parallel = 1
	}

	stats := &ExecutionStats{
		Total:     times,
		StartTime: time.Now(),
	}

	// (Ctrl+C)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\nReceived interrupt signal, cancelling...\n")
		cancel()
	}()

	if verbose && times > 1 {
		if parallel > 1 {
			fmt.Fprintf(os.Stderr, "Running %d requests (%d concurrent per batch)...\n", times, parallel)
		} else {
			fmt.Fprintf(os.Stderr, "Running %d requests sequentially...\n", times)
		}
	}

	batches := (times + parallel - 1) / parallel
	remaining := times
	completed := 0

	for batchNum := range batches {
		// Check for cancellation
		select {
		case <-ctx.Done():
			stats.EndTime = time.Now()
			if times > 1 {
				stats.Print()
			}
			return fmt.Errorf("execution cancelled")
		default:
		}

		if batchNum > 0 && delay > 0 {
			time.Sleep(time.Duration(delay) * time.Second)
		}

		// Calculate batch size (last batch may be smaller)
		batchSize := min(remaining, parallel)
		remaining -= batchSize

		if parallel > 1 {
			var wg sync.WaitGroup
			for range batchSize {
				wg.Add(1)
				go func() {
					defer wg.Done()

					// Check cancellation before executing
					select {
					case <-ctx.Done():
						return
					default:
					}

					if err := execShellCommand(cmdText); err != nil {
						stats.RecordFailure(err)
						if verbose {
							fmt.Fprintf(os.Stderr, "command execution failed: %v\n", err)
						}
					} else {
						stats.RecordSuccess()
					}
				}()
			}
			wg.Wait()
		} else {
			if err := execShellCommand(cmdText); err != nil {
				stats.RecordFailure(err)
				stats.EndTime = time.Now()
				if times > 1 {
					stats.Print()
				}
				return fmt.Errorf("command execution failed: %w", err)
			}
			stats.RecordSuccess()
		}

		completed += batchSize
		if verbose && times > 1 {
			fmt.Fprintf(os.Stderr, "Progress: %d/%d (%.1f%%)\n", completed, times, float64(completed)/float64(times)*100)
		}
	}

	stats.EndTime = time.Now()

	// Print summary for multiple requests
	if times > 1 && verbose {
		stats.Print()
	}

	return nil
}

func execShellCommand(cmdText string) error {
	execCmd := exec.Command("sh", "-c", cmdText)
	execCmd.Stdin = os.Stdin
	out, err := execCmd.CombinedOutput()

	// Lock to prevent output interleaving in parallel mode
	outputMutex.Lock()
	fmt.Printf("%s\n", string(out))
	outputMutex.Unlock()

	if err != nil {
		return fmt.Errorf("command exited with error: %w", err)
	}
	return nil
}

func runFile(filePath, dir, envName string, insecure bool) (string, error) {
	var envVars Environment
	if envName != "" {
		var err error
		envVars, err = loadEnvironmentVariables(envName, dir)
		if err != nil {
			return "", err
		}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)
	if envName != "" {
		contentStr = applyEnvironmentVars(contentStr, envVars)
	}

	if insecure {
		contentStr = strings.ReplaceAll(contentStr, "curl ", "curl -k ")
	}

	cmdText := extractShellCommand(contentStr)
	if cmdText == "" {
		return "", errors.New("no curl command found in file")
	}

	return cmdText, nil
}

func loadEnvConfig(filename string) (*EnvConfig, error) {
	data, err := os.ReadFile(filename)
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

		// Skip comments and empty lines at the beginning
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
