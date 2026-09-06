package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// AppVersion is shown on the startup banner.
const AppVersion = "2.0.0"

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiItalic = "\033[3m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"

	// Shared width for startup banner / warning / info panels.
	startupPanelInnerWidth = 92
)

// StartupIssue is one line item shown in a startup warning panel.
type StartupIssue struct {
	Path   string
	Label  string
	Reason string
}

// StartupCheck is one startup self-check row (success or failure).
type StartupCheck struct {
	Name   string
	OK     bool
	Detail string
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func colorize(enabled bool, code, text string) string {
	if !enabled || text == "" {
		return text
	}
	return code + text + ansiReset
}

func visibleWidth(s string) int {
	// Strip simple ANSI sequences for width calculation.
	plain := s
	for {
		start := strings.IndexByte(plain, '\033')
		if start < 0 {
			break
		}
		end := strings.IndexByte(plain[start:], 'm')
		if end < 0 {
			break
		}
		plain = plain[:start] + plain[start+end+1:]
	}
	return utf8.RuneCountInString(plain)
}

func wrapWords(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if width < 16 {
		width = 16
	}
	words := strings.Fields(text)
	var lines []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() == 0 {
			cur.WriteString(w)
			continue
		}
		if utf8.RuneCountInString(cur.String())+1+utf8.RuneCountInString(w) > width {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
			continue
		}
		cur.WriteByte(' ')
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

func printBoxLine(colorEnabled bool, borderColor, content string, innerWidth int) {
	pad := innerWidth - visibleWidth(content)
	if pad < 0 {
		pad = 0
	}
	left := colorize(colorEnabled, borderColor, "│")
	right := colorize(colorEnabled, borderColor, "│")
	fmt.Printf("%s %s%s %s\n", left, content, strings.Repeat(" ", pad), right)
}

func printBoxTop(colorEnabled bool, borderColor string, innerWidth int) {
	fmt.Println(colorize(colorEnabled, borderColor, "┌"+strings.Repeat("─", innerWidth+2)+"┐"))
}

func printBoxSep(colorEnabled bool, borderColor string, innerWidth int) {
	fmt.Println(colorize(colorEnabled, borderColor, "├"+strings.Repeat("─", innerWidth+2)+"┤"))
}

func printBoxBottom(colorEnabled bool, borderColor string, innerWidth int) {
	fmt.Println(colorize(colorEnabled, borderColor, "└"+strings.Repeat("─", innerWidth+2)+"┘"))
}

// PrintStartupWarningPanel renders a colored warning box (TTY) or plain text fallback.
func PrintStartupWarningPanel(title string, intro string, issues []StartupIssue, footer string) {
	color := stdoutIsTTY()
	border := ansiYellow
	innerWidth := startupPanelInnerWidth

	fmt.Println()
	printBoxTop(color, border, innerWidth)
	printBoxLine(color, border, colorize(color, ansiBold+ansiYellow, "⚠  "+title), innerWidth)
	printBoxSep(color, border, innerWidth)

	for _, line := range wrapWords(intro, innerWidth) {
		printBoxLine(color, border, line, innerWidth)
	}
	if intro != "" {
		printBoxLine(color, border, "", innerWidth)
	}

	for i, issue := range issues {
		bullet := colorize(color, ansiRed+ansiBold, "•")
		path := colorize(color, ansiBold, issue.Path)
		printBoxLine(color, border, fmt.Sprintf("%s %s", bullet, path), innerWidth)
		detail := issue.Label
		if issue.Reason != "" {
			if detail != "" {
				detail += " — "
			}
			detail += issue.Reason
		}
		for _, line := range wrapWords(detail, innerWidth-2) {
			printBoxLine(color, border, "  "+colorize(color, ansiDim, line), innerWidth)
		}
		if i != len(issues)-1 {
			printBoxLine(color, border, "", innerWidth)
		}
	}

	if footer != "" {
		printBoxSep(color, border, innerWidth)
		for _, line := range wrapWords(footer, innerWidth) {
			printBoxLine(color, border, colorize(color, ansiDim, line), innerWidth)
		}
	}
	printBoxBottom(color, border, innerWidth)
	fmt.Println()
}

// PrintStartupChecksPanel renders a summary box for startup self-checks.
// Uses a green border when all checks pass, otherwise yellow.
func PrintStartupChecksPanel(title string, checks []StartupCheck) {
	if len(checks) == 0 {
		return
	}
	if strings.TrimSpace(title) == "" {
		title = "Startup checks"
	}
	color := stdoutIsTTY()
	allOK := true
	for _, c := range checks {
		if !c.OK {
			allOK = false
			break
		}
	}
	border := ansiGreen
	titleColor := ansiBold + ansiGreen
	titlePrefix := "✔  "
	if !allOK {
		border = ansiYellow
		titleColor = ansiBold + ansiYellow
		titlePrefix = "⚠  "
	}
	innerWidth := startupPanelInnerWidth

	fmt.Println()
	printBoxTop(color, border, innerWidth)
	printBoxLine(color, border, colorize(color, titleColor, titlePrefix+title), innerWidth)
	printBoxSep(color, border, innerWidth)
	for _, c := range checks {
		mark := colorize(color, ansiGreen+ansiBold, "✔")
		if !c.OK {
			mark = colorize(color, ansiRed+ansiBold, "✖")
		}
		name := colorize(color, ansiBold, c.Name)
		line := fmt.Sprintf("%s %s", mark, name)
		if strings.TrimSpace(c.Detail) != "" {
			line = fmt.Sprintf("%s  %s", line, colorize(color, ansiDim, c.Detail))
		}
		// If the combined line is too wide, print name then wrapped detail.
		if visibleWidth(line) <= innerWidth {
			printBoxLine(color, border, line, innerWidth)
			continue
		}
		printBoxLine(color, border, fmt.Sprintf("%s %s", mark, name), innerWidth)
		for _, w := range wrapWords(c.Detail, innerWidth-4) {
			printBoxLine(color, border, "    "+colorize(color, ansiDim, w), innerWidth)
		}
	}
	printBoxBottom(color, border, innerWidth)
	fmt.Println()
}

// PrintStartupInfoPanel renders a cyan info box for non-fatal prompts (e.g. Docker pull).
func PrintStartupInfoPanel(title string, lines []string) {
	color := stdoutIsTTY()
	border := ansiCyan
	innerWidth := startupPanelInnerWidth

	fmt.Println()
	printBoxTop(color, border, innerWidth)
	printBoxLine(color, border, colorize(color, ansiBold+ansiCyan, "ℹ  "+title), innerWidth)
	printBoxSep(color, border, innerWidth)
	for _, raw := range lines {
		if strings.TrimSpace(raw) == "" {
			printBoxLine(color, border, "", innerWidth)
			continue
		}
		for _, line := range wrapWords(raw, innerWidth) {
			printBoxLine(color, border, line, innerWidth)
		}
	}
	printBoxBottom(color, border, innerWidth)
	fmt.Println()
}

// AskYesNo prints a prompt and returns true only when the user answers y/yes.
func AskYesNo(prompt string) bool {
	color := stdoutIsTTY()
	fmt.Print(colorize(color, ansiBold, prompt))
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(answer) == "" {
		fmt.Println()
		fmt.Println(colorize(color, ansiDim, "No confirmation received."))
		return false
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// PrintStartupCancelled prints a short cancellation note.
func PrintStartupCancelled(message string) {
	color := stdoutIsTTY()
	fmt.Println(colorize(color, ansiRed, "✖  "+message))
}

// PrintStartupOK prints a short success/ack note.
func PrintStartupOK(message string) {
	color := stdoutIsTTY()
	fmt.Println(colorize(color, ansiGreen, "✔  "+message))
}

// PrintAppBanner prints a startup panel matched to the security-warning box
// (same width / frame language), with a larger wordmark and runtime meta.
func PrintAppBanner(version string, port int, ginMode string) {
	if strings.TrimSpace(version) == "" {
		version = AppVersion
	}
	if strings.TrimSpace(ginMode) == "" {
		ginMode = "release"
	}

	color := stdoutIsTTY()
	border := ansiCyan
	innerWidth := startupPanelInnerWidth

	// Slanted "FISHING" wordmark (visual italic — many terminals ignore ANSI italic).
	art := []string{
		`    _______________ __  _______   ________`,
		`   / ____/  _/ ___// / / /  _/ | / / ____/`,
		`  / /_   / / \__ \/ /_/ // //  |/ / / __  `,
		` / __/ _/ / ___/ / __  // // /|  / /_/ /  `,
		`/_/   /___//____/_/ /_/___/_/ |_/\____/   `,
	}

	fmt.Println()
	printBoxTop(color, border, innerWidth)
	printBoxLine(color, border, "", innerWidth)
	printBoxLine(color, border, "", innerWidth)
	for _, line := range art {
		printBoxLine(color, border, colorize(color, ansiBold+ansiItalic+ansiCyan, centerText(line, innerWidth)), innerWidth)
	}
	printBoxLine(color, border, "", innerWidth)
	printBoxLine(color, border, colorize(color, ansiBold, centerText("PLATFORM", innerWidth)), innerWidth)
	printBoxLine(color, border, "", innerWidth)
	printBoxLine(color, border, "", innerWidth)
	printBoxSep(color, border, innerWidth)
	printBoxLine(color, border, "", innerWidth)
	printBoxLine(color, border, colorize(color, ansiBold, fmt.Sprintf("  Version   %s", version)), innerWidth)
	printBoxLine(color, border, colorize(color, ansiBold, fmt.Sprintf("  Listen    :%d", port)), innerWidth)
	printBoxLine(color, border, colorize(color, ansiBold, fmt.Sprintf("  Mode      %s", ginMode)), innerWidth)
	printBoxLine(color, border, colorize(color, ansiDim, "  Credential capture operations console"), innerWidth)
	printBoxLine(color, border, "", innerWidth)
	printBoxBottom(color, border, innerWidth)
	fmt.Println()
}

func centerText(s string, width int) string {
	w := visibleWidth(s)
	if w >= width {
		return s
	}
	pad := width - w
	left := pad / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", pad-left)
}
