package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
)

// Constants for configuration and timeouts
const (
	DefaultSchedule     = "10s"
	DefaultTCPTimeout   = 5 * time.Second
	DefaultSleepBackoff = 10 * time.Second
	DefaultTCPPort      = "80"
	HTTPSScheme         = "https://"

	// Time format layouts
	DisplayTimeFormat = "2006-01-02 15:04:05 MST"

	// Common timestamp field names
	TimestampField1 = "timestamp"
	TimestampField2 = "time"
	TimestampField3 = "date"
)

// --- Styles ---
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")). // bright white
			Background(lipgloss.Color("0")).  // black
			Padding(0, 2).
			MarginBottom(1)

	sectionTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("7")). // light gray
			PaddingRight(1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")) // green

	sectionBox = lipgloss.NewStyle().
			PaddingLeft(0).
			PaddingRight(0)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")). // red
			Bold(true).
			Padding(0, 1).
			MarginTop(1)

	healthKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")). // light gray
			Bold(true)

	healthValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")) // white

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")). // gray
			Padding(1, 0).
			MarginTop(1)
)

// --- Helper functions for rendering sections ---
func renderSection(title, value string) string {
	return fmt.Sprintf("%s %s", sectionTitle.Render(title), infoStyle.Render(value))
}

func renderHealthSection(data map[string]any, tz *time.Location) string {
	var lines []string
	lines = append(lines, sectionTitle.Render("Health Endpoint:"))
	for k, v := range data {
		if s, ok := v.(string); ok && (k == TimestampField1 || k == TimestampField2 || k == TimestampField3) {
			if tz != nil {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					s = t.In(tz).Format(DisplayTimeFormat)
				}
			}
			lines = append(lines, fmt.Sprintf("  %s %s", healthKeyStyle.Render(k+":"), healthValueStyle.Render(s)))
		} else {
			lines = append(lines, fmt.Sprintf("  %s %s", healthKeyStyle.Render(k+":"), healthValueStyle.Render(fmt.Sprintf("%v", v))))
		}
	}
	return strings.Join(lines, "\n")
}

// --- Bubble Tea Model Methods ---
func (m model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.websites))
	for i, website := range m.websites {
		cmds[i] = pingWebsiteCmdWithContext(m.ctx, website, i)
	}
	return tea.Batch(cmds...)
}

func schedulePing(schedule string, idx int) tea.Cmd {
	dur, err := time.ParseDuration(schedule)
	if err != nil {
		dur = DefaultSleepBackoff
	}
	return func() tea.Msg {
		time.Sleep(dur)
		return tickMsgWithIndex{Time: time.Now(), Index: idx}
	}
}

type tickMsgWithIndex struct {
	Time  time.Time
	Index int
}

func pingWebsiteCmdWithContext(ctx context.Context, website string, idx int) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		dialer := &net.Dialer{Timeout: DefaultTCPTimeout}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(website, DefaultTCPPort))
		if err != nil {
			return pingResultWithIndex{Result: "", Err: err, Index: idx}
		}
		_ = conn.Close()
		elapsed := time.Since(start)
		result := fmt.Sprintf(
			"Ping to %s:\n  TCP connect successful\n  Time: %v ms",
			website,
			elapsed.Milliseconds(),
		)
		return pingResultWithIndex{Result: result, Err: nil, Index: idx}
	}
}

func fetchHealthCmdWithContext(ctx context.Context, website, healthEndpoint string, idx int) tea.Cmd {
	return func() tea.Msg {
		if healthEndpoint == "" {
			return healthResultGenericWithIndex{Data: nil, Err: fmt.Errorf("health endpoint not configured"), Index: idx}
		}
		url := HTTPSScheme + website + healthEndpoint
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return healthResultGenericWithIndex{Data: nil, Err: err, Index: idx}
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return healthResultGenericWithIndex{Data: nil, Err: err, Index: idx}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return healthResultGenericWithIndex{Data: nil, Err: err, Index: idx}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return healthResultGenericWithIndex{Data: nil, Err: fmt.Errorf("health endpoint HTTP %d: %s", resp.StatusCode, string(body)), Index: idx}
		}
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			return healthResultGenericWithIndex{Data: nil, Err: fmt.Errorf("invalid JSON from health endpoint: %w\nBody: %s", err, string(body)), Index: idx}
		}
		return healthResultGenericWithIndex{Data: data, Err: nil, Index: idx}
	}
}

type pingResultWithIndex struct {
	Result string
	Err    error
	Index  int
}

type healthResultGenericWithIndex struct {
	Data  map[string]any
	Err   error
	Index int
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsgWithIndex:
		return m, pingWebsiteCmdWithContext(m.ctx, m.websites[msg.Index], msg.Index)
	case pingResultWithIndex:
		if msg.Err != nil {
			m.lastError[msg.Index] = msg.Err.Error()
			m.lastPing[msg.Index] = ""
			m.lastHealthGeneric[msg.Index] = nil
			return m, schedulePing(m.schedule, msg.Index)
		}
		m.lastPing[msg.Index] = msg.Result
		m.lastError[msg.Index] = ""
		// Use per-website health endpoint
		if len(m.healthEndpoint) > msg.Index && m.healthEndpoint[msg.Index] != "" {
			return m, fetchHealthCmdWithContext(m.ctx, m.websites[msg.Index], m.healthEndpoint[msg.Index], msg.Index)
		}
		return m, schedulePing(m.schedule, msg.Index)
	case healthResultGenericWithIndex:
		if msg.Err == nil {
			m.lastHealthGeneric[msg.Index] = msg.Data
		} else {
			m.lastHealthGeneric[msg.Index] = nil
			m.lastError[msg.Index] = msg.Err.Error()
		}
		return m, schedulePing(m.schedule, msg.Index)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			m.quit = true
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render(" Vivteno - Website Health Monitor "))
	b.WriteString("\n\n")

	// For each website, render its section
	for i, website := range m.websites {
		b.WriteString(renderSection("Website:", website))
		b.WriteString("\n")
		b.WriteString(renderSection("Schedule:", m.schedule))
		b.WriteString("\n")

		// Ping Section
		if m.lastPing[i] != "" {
			now := time.Now()
			if m.timezone != nil {
				now = now.In(m.timezone)
			}
			b.WriteString("\n")
			b.WriteString(renderSection("Last checked:", now.Format(DisplayTimeFormat)))
			b.WriteString("\n")
			for j, line := range strings.Split(m.lastPing[i], "\n") {
				if j == 0 {
					b.WriteString(infoStyle.Render(line))
				} else {
					b.WriteString("\n" + infoStyle.Render(line))
				}
			}
			b.WriteString("\n")
		}

		// Health Endpoint Section
		if len(m.healthEndpoint) > i && m.healthEndpoint[i] != "" && m.lastHealthGeneric[i] != nil {
			b.WriteString("\n")
			b.WriteString(renderHealthSection(m.lastHealthGeneric[i], m.timezone))
			b.WriteString("\n")
		}

		// Error Section
		if m.lastError[i] != "" {
			b.WriteString("\n")
			b.WriteString(errorStyle.Render("FAILED: " + m.lastError[i]))
			b.WriteString("\n")
		}

		if len(m.websites) > 1 && i < len(m.websites)-1 {
			b.WriteString("\n" + strings.Repeat("-", 40) + "\n\n")
		}
	}

	// Footer
	b.WriteString(footerStyle.Render("Press q or Ctrl+C to quit."))

	return b.String()
}

// --- Validation helpers ---
func isValidHostname(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	re := regexp.MustCompile(`^([a-zA-Z0-9-]+\.)*[a-zA-Z0-9-]+$`)
	return re.MatchString(host)
}

func isValidSchedule(s string) bool {
	_, err := time.ParseDuration(s)
	return err == nil
}

// --- Main entrypoint ---
func main() {
	_ = godotenv.Load()
	websiteEnv := os.Getenv("PING_WEBSITE")
	schedule := os.Getenv("PING_SCHEDULE")
	timezone := os.Getenv("TIMEZONE")
	healthEndpointEnv := os.Getenv("HEALTH_ENDPOINT")

	var websites []string
	if err := json.Unmarshal([]byte(websiteEnv), &websites); err != nil || len(websites) == 0 {
		fmt.Println("PING_WEBSITE must be a JSON array of at least one website, e.g. [\"example.com\"]")
		os.Exit(1)
	}
	for _, w := range websites {
		if !isValidHostname(w) {
			fmt.Printf("Invalid website in PING_WEBSITE: %q\n", w)
			os.Exit(1)
		}
	}
	if schedule == "" {
		schedule = DefaultSchedule
	}
	if !isValidSchedule(schedule) {
		fmt.Printf("Invalid PING_SCHEDULE: %q\n", schedule)
		os.Exit(1)
	}

	var loc *time.Location
	var err error
	if timezone != "" {
		loc, err = time.LoadLocation(timezone)
		if err != nil {
			fmt.Printf("Invalid TIMEZONE: %q\n", timezone)
			os.Exit(1)
		}
	} else {
		loc = time.Local
	}

	// Parse HEALTH_ENDPOINT as array or fallback to single value for all
	var healthEndpoints []string
	if healthEndpointEnv != "" {
		// Try to parse as JSON array
		if err := json.Unmarshal([]byte(healthEndpointEnv), &healthEndpoints); err != nil {
			// fallback: treat as single endpoint for all
			healthEndpoints = make([]string, len(websites))
			for i := range healthEndpoints {
				healthEndpoints[i] = healthEndpointEnv
			}
		} else if len(healthEndpoints) != len(websites) {
			fmt.Println("HEALTH_ENDPOINT must be a JSON array with the same length as PING_WEBSITE, or a single string.")
			os.Exit(1)
		}
	} else {
		healthEndpoints = make([]string, len(websites))
	}

	ctx, cancel := context.WithCancel(context.Background())
	m := initialModel(websites, schedule, healthEndpoints, ctx, cancel)
	m.timezone = loc
	p := tea.NewProgram(m)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		cancel()
		p.Quit()
	}()
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
