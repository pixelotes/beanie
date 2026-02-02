package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// DATA STRUCTURES - Pipeline-based configuration
// ============================================================================

type Config struct {
	Tools []ToolDefinition `yaml:"tools"`
}

type ToolDefinition struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Args        map[string]ArgProps `yaml:"args"`
	Pipeline    []PipelineStep      `yaml:"pipeline"`
}

type ArgProps struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

type PipelineStep struct {
	Type    string            `yaml:"type"`    // "native" or "exec"
	Command string            `yaml:"command"` // Go function name or shell command
	Args    map[string]string `yaml:"args"`    // Arguments with {{.var}} templating
}

// ============================================================================
// NATIVE FUNCTION REGISTRY
// ============================================================================

// NativeFunc is the signature for all internal Go functions
type NativeFunc func(ctx context.Context, input string, args map[string]string) (string, error)

// nativeRegistry maps command names to their Go implementations
var nativeRegistry = map[string]NativeFunc{
	"text_trim":       nativeTextTrim,
	"text_replace":    nativeTextReplace,
	"http_get":        nativeHttpGet,
	"fs_read":         nativeFsRead,
	"fs_write":        nativeFsWrite,
	"gzip_decompress": nativeGzipDecompress,
	"html_strip_tags": nativeHtmlStripTags,
	"dice_roll":       nativeDiceRoll,
	"api_call":        nativeApiCall,
	"json_query":      nativeJsonQuery,
}

// --- Native Function Implementations ---

func nativeTextTrim(ctx context.Context, input string, args map[string]string) (string, error) {
	return strings.TrimSpace(input), nil
}

func nativeTextReplace(ctx context.Context, input string, args map[string]string) (string, error) {
	old := args["old"]
	new := args["new"]
	if old == "" {
		return input, nil
	}
	return strings.ReplaceAll(input, old, new), nil
}

func nativeHttpGet(ctx context.Context, input string, args map[string]string) (string, error) {
	target := args["url"]
	if target == "" {
		target = strings.TrimSpace(input)
	}
	if target == "" {
		return "", fmt.Errorf("http_get: no URL provided")
	}

	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		return "", fmt.Errorf("http_get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("http_get: reading body: %w", err)
	}

	return string(body), nil
}

func nativeFsRead(ctx context.Context, input string, args map[string]string) (string, error) {
	path := args["path"]
	if path == "" {
		path = strings.TrimSpace(input)
	}
	if path == "" {
		return "", fmt.Errorf("fs_read: no path provided")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("fs_read: %w", err)
	}

	return string(data), nil
}

func nativeFsWrite(ctx context.Context, input string, args map[string]string) (string, error) {
	path := args["path"]
	if path == "" {
		return "", fmt.Errorf("fs_write: no path provided")
	}

	content := input
	if args["content"] != "" {
		content = args["content"]
	}

	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("fs_write: %w", err)
	}

	return fmt.Sprintf("Written %d bytes to %s", len(content), path), nil
}

func nativeGzipDecompress(ctx context.Context, input string, args map[string]string) (string, error) {
	reader, err := gzip.NewReader(strings.NewReader(input))
	if err != nil {
		return "", fmt.Errorf("gzip_decompress: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("gzip_decompress: reading: %w", err)
	}

	return string(decompressed), nil
}

func nativeHtmlStripTags(ctx context.Context, input string, args map[string]string) (string, error) {
	// Remove script and style elements entirely
	reScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reStyle := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	result := reScript.ReplaceAllString(input, "")
	result = reStyle.ReplaceAllString(result, "")

	// Remove all HTML tags
	reTags := regexp.MustCompile(`<[^>]+>`)
	result = reTags.ReplaceAllString(result, "\n")

	// Decode common HTML entities
	result = strings.ReplaceAll(result, "&nbsp;", " ")
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&#39;", "'")

	// Clean up whitespace: collapse multiple newlines/spaces
	reMultiNewline := regexp.MustCompile(`\n{3,}`)
	result = reMultiNewline.ReplaceAllString(result, "\n\n")

	reMultiSpace := regexp.MustCompile(`[ \t]+`)
	result = reMultiSpace.ReplaceAllString(result, " ")

	// Trim each line
	lines := strings.Split(result, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}

	return strings.Join(cleaned, "\n"), nil
}

// nativeDiceRoll parses and rolls dice notation like "2d12+1d6+1"
// Supports: NdM (N dice with M sides), +/- operators, numeric modifiers
func nativeDiceRoll(ctx context.Context, input string, args map[string]string) (string, error) {
	notation := args["notation"]
	if notation == "" {
		notation = strings.TrimSpace(input)
	}
	if notation == "" {
		return "", fmt.Errorf("dice_roll: no notation provided (e.g., '2d6+1')")
	}

	// Seed random (only needed once, but safe to call multiple times)
	rand.Seed(time.Now().UnixNano())

	// Regex to parse dice terms: captures sign, count, sides, or just a number
	// Examples: "2d12", "+1d6", "-3", "+5"
	termRegex := regexp.MustCompile(`([+-]?)(\d+)(?:d(\d+))?`)
	matches := termRegex.FindAllStringSubmatch(notation, -1)
	
	if len(matches) == 0 {
		return "", fmt.Errorf("dice_roll: invalid notation '%s'", notation)
	}

	var total int
	var details []string

	for i, match := range matches {
		sign := 1
		if match[1] == "-" {
			sign = -1
		}

		count, _ := strconv.Atoi(match[2])
		
		if match[3] != "" {
			// It's a dice roll: NdM
			sides, _ := strconv.Atoi(match[3])
			if sides < 1 {
				return "", fmt.Errorf("dice_roll: invalid dice 'd%d'", sides)
			}
			if count < 1 || count > 100 {
				return "", fmt.Errorf("dice_roll: count must be 1-100, got %d", count)
			}
			if sides > 1000 {
				return "", fmt.Errorf("dice_roll: max 1000 sides, got %d", sides)
			}

			var rolls []int
			subtotal := 0
			for j := 0; j < count; j++ {
				roll := rand.Intn(sides) + 1
				rolls = append(rolls, roll)
				subtotal += roll
			}

			// Format: "2d12: [7, 11] = 18"
			rollStrs := make([]string, len(rolls))
			for j, r := range rolls {
				rollStrs[j] = strconv.Itoa(r)
			}
			
			signStr := ""
			if i > 0 {
				if sign == 1 {
					signStr = "+ "
				} else {
					signStr = "- "
				}
			} else if sign == -1 {
				signStr = "-"
			}
			
			details = append(details, fmt.Sprintf("%s%dd%d: [%s] = %d", 
				signStr, count, sides, strings.Join(rollStrs, ", "), subtotal))
			total += sign * subtotal
		} else {
			// It's a plain modifier
			signStr := ""
			if i > 0 {
				if sign == 1 {
					signStr = "+ "
				} else {
					signStr = "- "
				}
			} else if sign == -1 {
				signStr = "-"
			}
			
			details = append(details, fmt.Sprintf("%s%d", signStr, count))
			total += sign * count
		}
	}

	// Build result
	result := fmt.Sprintf("🎲 %s\n\n%s\n\n**Total: %d**", 
		notation, 
		strings.Join(details, "\n"), 
		total)

	return result, nil
}

// nativeApiCall makes HTTP API calls with configurable method, headers, and body
// Supports environment variable substitution using ${VAR_NAME} syntax for API keys
func nativeApiCall(ctx context.Context, input string, args map[string]string) (string, error) {
	// 1. Get and expand URL
	urlStr := expandEnvVars(args["url"])
	if urlStr == "" {
		return "", fmt.Errorf("api_call: 'url' argument is required")
	}

	// 2. HTTP Method (default: GET)
	method := strings.ToUpper(args["method"])
	if method == "" {
		method = "GET"
	}
	validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true, "HEAD": true}
	if !validMethods[method] {
		return "", fmt.Errorf("api_call: invalid method '%s'", method)
	}

	// 3. Parse headers from JSON string
	headers := make(map[string]string)
	if h := args["headers"]; h != "" {
		h = expandEnvVars(h)
		if err := json.Unmarshal([]byte(h), &headers); err != nil {
			return "", fmt.Errorf("api_call: invalid headers JSON: %w", err)
		}
	}

	// 4. Determine request body
	var bodyReader io.Reader
	if args["body_from_input"] == "true" && input != "" {
		bodyReader = strings.NewReader(input)
	} else if b := args["body"]; b != "" {
		bodyReader = strings.NewReader(b)
	}

	// 5. Create HTTP request
	req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
	if err != nil {
		return "", fmt.Errorf("api_call: creating request: %w", err)
	}

	// 6. Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Default Content-Type for POST/PUT/PATCH if body exists and not set
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		if method == "POST" || method == "PUT" || method == "PATCH" {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	// 7. Timeout
	timeout := 30 * time.Second
	if t := args["timeout"]; t != "" {
		if secs, err := strconv.Atoi(t); err == nil && secs > 0 {
			timeout = time.Duration(secs) * time.Second
		}
	}

	client := &http.Client{Timeout: timeout}

	// 8. Execute request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("api_call: request failed: %w", err)
	}
	defer resp.Body.Close()

	// 9. Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("api_call: reading response: %w", err)
	}

	// 10. Check for error status codes
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("api_call: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

// expandEnvVars replaces ${VAR_NAME} with the value of environment variable VAR_NAME
func expandEnvVars(s string) string {
	re := regexp.MustCompile(`\$\{(\w+)\}`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1] // Extract VAR_NAME from ${VAR_NAME}
		return os.Getenv(varName)
	})
}

// nativeJsonQuery uses jq binary to extract/transform JSON data
// Input: JSON string from previous pipeline step
// Args:
//   - filter: jq filter expression (required), e.g., ".data.items[].name", ".[] | select(.active)"
//   - raw: if "true", use -r flag for raw string output (no quotes)
func nativeJsonQuery(ctx context.Context, input string, args map[string]string) (string, error) {
	filter := args["filter"]
	if filter == "" {
		filter = "."
	}

	// Build jq command
	jqArgs := []string{}
	
	// Raw output mode (no quotes around strings)
	if args["raw"] == "true" {
		jqArgs = append(jqArgs, "-r")
	}
	
	jqArgs = append(jqArgs, filter)

	cmd := exec.CommandContext(ctx, "jq", jqArgs...)
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Include stderr for debugging jq syntax errors
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("json_query: jq error: %s", errMsg)
		}
		return "", fmt.Errorf("json_query: %w", err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// ============================================================================
// MAIN - Server Setup
// ============================================================================

func main() {
	// 1. Detect binary directory for relative paths
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Cannot determine executable path: %v\n", err)
		os.Exit(1)
	}
	exeDir := filepath.Dir(exePath)

	// 2. Load tools.yaml
	configPath := filepath.Join(exeDir, "tools.yaml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Cannot read tools.yaml at: %s\nError: %v\n", configPath, err)
		os.Exit(1)
	}

	var config Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: YAML parse error: %v\n", err)
		os.Exit(1)
	}

	// 3. Create MCP Server
	s := server.NewMCPServer("pixelotes-pi-server", "3.0.0", server.WithLogging())

	// 4. Register tools dynamically from YAML
	for _, toolDef := range config.Tools {
		t := toolDef // Capture for closure

		// Build tool options
		var toolOptions []mcp.ToolOption
		toolOptions = append(toolOptions, mcp.WithDescription(t.Description))

		for argName, argProps := range t.Args {
			toolOptions = append(toolOptions, mcp.WithString(
				argName,
				mcp.Description(argProps.Description),
				mcp.Required(),
			))
		}

		mcpTool := mcp.NewTool(t.Name, toolOptions...)

		// Add handler with pipeline executor
		s.AddTool(mcpTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return executePipeline(ctx, t, request, exeDir)
		})
	}

	// 5. Start server
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
	}
}

// ============================================================================
// PIPELINE ENGINE - The core orchestrator
// ============================================================================

func executePipeline(ctx context.Context, toolDef ToolDefinition, req mcp.CallToolRequest, baseDir string) (*mcp.CallToolResult, error) {
	// Build template data from request arguments
	templateData := make(map[string]interface{})
	for k, v := range req.Params.Arguments {
		templateData[k] = v
	}

	// Pipeline state: output of previous step becomes input of next
	var currentInput string

	// Execute each step in sequence
	for stepIdx, step := range toolDef.Pipeline {
		// Resolve template variables in step arguments
		resolvedArgs, err := resolveTemplateArgs(step.Args, templateData)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Step %d template error: %v", stepIdx+1, err)), nil
		}

		// Resolve template in command string
		resolvedCommand, err := resolveTemplateString(step.Command, templateData)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Step %d command template error: %v", stepIdx+1, err)), nil
		}

		var output string

		switch step.Type {
		case "native":
			fn, exists := nativeRegistry[resolvedCommand]
			if !exists {
				return mcp.NewToolResultError(fmt.Sprintf("Step %d: unknown native function '%s'", stepIdx+1, resolvedCommand)), nil
			}
			output, err = fn(ctx, currentInput, resolvedArgs)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Step %d (%s) error: %v", stepIdx+1, resolvedCommand, err)), nil
			}

		case "exec":
			output, err = executeShellCommand(ctx, resolvedCommand, currentInput, baseDir)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Step %d (exec) error: %v\nOutput: %s", stepIdx+1, err, output)), nil
			}

		default:
			return mcp.NewToolResultError(fmt.Sprintf("Step %d: unknown type '%s'", stepIdx+1, step.Type)), nil
		}

		// Pass output to next step
		currentInput = output
	}

	// Truncate if too long
	if len(currentInput) > 50000 {
		currentInput = currentInput[:50000] + "\n...[Truncated]..."
	}

	return mcp.NewToolResultText(currentInput), nil
}

// resolveTemplateArgs applies Go templating to all values in an args map
func resolveTemplateArgs(args map[string]string, data map[string]interface{}) (map[string]string, error) {
	resolved := make(map[string]string)
	for k, v := range args {
		result, err := resolveTemplateString(v, data)
		if err != nil {
			return nil, fmt.Errorf("arg '%s': %w", k, err)
		}
		resolved[k] = result
	}
	return resolved, nil
}

// resolveTemplateString applies Go templating to a single string
func resolveTemplateString(tmplStr string, data map[string]interface{}) (string, error) {
	if !strings.Contains(tmplStr, "{{") {
		return tmplStr, nil
	}

	tmpl, err := template.New("step").Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// executeShellCommand runs a shell command with stdin injection
func executeShellCommand(ctx context.Context, command string, stdin string, baseDir string) (string, error) {
	// Expand scripts/ to absolute path
	if strings.Contains(command, "scripts/") {
		absoluteScriptsPath := filepath.Join(baseDir, "scripts")
		command = strings.Replace(command, "scripts", absoluteScriptsPath, 1)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = baseDir

	// Inject previous step's output as stdin
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}
