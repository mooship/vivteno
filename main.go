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

// --- Styles (defined once, reused) ---
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

func renderHealthSection(endpoint string, data map[string]any, tz *time.Location) string {
	var lines []string
	lines = append(lines, sectionTitle.Render("Health Endpoint:"))
	for k, v := range data {
		if s, ok := v.(string); ok && (k == "timestamp" || k == "time" || k == "date") {
			if tz != nil {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					s = t.In(tz).Format("2006-01-02 15:04:05 MST")
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
	return pingWebsiteCmdWithContext(m.ctx, m.website)
}

func schedulePing(schedule string) tea.Cmd {
	dur, err := time.ParseDuration(schedule)
	if err != nil {
		dur = 10 * time.Second
	}
	return func() tea.Msg {
		time.Sleep(dur)
		return tickMsg(time.Now())
	}
}

func pingWebsiteCmdWithContext(ctx context.Context, website string) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		dialer := &net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(website, "80"))
		if err != nil {
			return pingResult{"", err}
		}
		_ = conn.Close()
		elapsed := time.Since(start)
		result := fmt.Sprintf(
			"Ping to %s:\n  TCP connect successful\n  Time: %v ms",
			website,
			elapsed.Milliseconds(),
		)
		return pingResult{result, nil}
	}
}

func fetchHealthCmdWithContext(ctx context.Context, website, healthEndpoint string) tea.Cmd {
	return func() tea.Msg {
		if healthEndpoint == "" {
			return healthResultGeneric{nil, fmt.Errorf("health endpoint not configured")}
		}
		url := "https://" + website + healthEndpoint
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return healthResultGeneric{nil, err}
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return healthResultGeneric{nil, err}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return healthResultGeneric{nil, err}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return healthResultGeneric{nil, fmt.Errorf("health endpoint HTTP %d: %s", resp.StatusCode, string(body))}
		}
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			return healthResultGeneric{nil, fmt.Errorf("invalid JSON from health endpoint: %w\nBody: %s", err, string(body))}
		}
		return healthResultGeneric{data, nil}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, pingWebsiteCmdWithContext(m.ctx, m.website)
	case pingResult:
		if msg.err != nil {
			m.lastError = msg.err.Error()
			m.lastPing = ""
			m.lastHealthGeneric = nil
			return m, schedulePing(m.schedule)
		}
		m.lastPing = msg.result
		m.lastError = ""
		if m.healthEndpoint != "" {
			return m, fetchHealthCmdWithContext(m.ctx, m.website, m.healthEndpoint)
		}
		return m, schedulePing(m.schedule)
	case healthResultGeneric:
		if msg.err == nil {
			m.lastHealthGeneric = msg.data
		} else {
			m.lastHealthGeneric = nil
			m.lastError = msg.err.Error()
		}
		return m, schedulePing(m.schedule)
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

	// Target & Schedule Section
	b.WriteString(renderSection("Website:", m.website))
	b.WriteString("\n")
	b.WriteString(renderSection("Schedule:", m.schedule))
	b.WriteString("\n")

	// Ping Section
	if m.lastPing != "" {
		now := time.Now()
		if m.timezone != nil {
			now = now.In(m.timezone)
		}
		b.WriteString("\n")
		b.WriteString(renderSection("Last checked:", now.Format("2006-01-02 15:04:05 MST")))
		b.WriteString("\n")
		// Ping result in green, multiline
		for i, line := range strings.Split(m.lastPing, "\n") {
			if i == 0 {
				b.WriteString(infoStyle.Render(line))
			} else {
				b.WriteString("\n" + infoStyle.Render(line))
			}
		}
		b.WriteString("\n")
	}

	// Health Endpoint Section
	if m.healthEndpoint != "" && m.lastHealthGeneric != nil {
		b.WriteString("\n")
		b.WriteString(renderHealthSection(m.healthEndpoint, m.lastHealthGeneric, m.timezone))
		b.WriteString("\n")
	}

	// Error Section
	if m.lastError != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("FAILED: " + m.lastError))
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
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
	website := os.Getenv("PING_WEBSITE")
	schedule := os.Getenv("PING_SCHEDULE")
	timezone := os.Getenv("TIMEZONE")
	healthEndpoint := os.Getenv("HEALTH_ENDPOINT")

	if website == "" {
		fmt.Println("PING_WEBSITE not set in .env")
		os.Exit(1)
	}
	if !isValidHostname(website) {
		fmt.Printf("Invalid PING_WEBSITE: %q\n", website)
		os.Exit(1)
	}
	if schedule == "" {
		schedule = "10s"
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

	ctx, cancel := context.WithCancel(context.Background())
	m := initialModel(website, schedule, ctx, cancel)
	m.timezone = loc
	m.healthEndpoint = healthEndpoint
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
