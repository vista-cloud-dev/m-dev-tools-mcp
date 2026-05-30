package clikit

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"golang.org/x/term"
)

// This file is the styling toolkit: an adaptive, semantic palette, a glyph set
// (with an ASCII fallback), and a set of composable primitives — titles,
// status lines, badges, key/value lists, panels, rules, trees, and tables.
//
// Everything here is a method on *Context and a no-op unless c.Color is set,
// so styled output appears only on an interactive TTY; piped/JSON output stays
// plain. Tune the palette below and every CLI in the toolchain inherits it.

// --- palette -----------------------------------------------------------------

// The palette is semantic (named by meaning, not hue) and adaptive: lipgloss
// picks the Light or Dark value from the detected terminal background, then
// downsamples to the terminal's color profile (truecolor → 256 → 16). One set
// of inks that reads well on a dark or a light terminal.
var (
	cIndigo = lipgloss.AdaptiveColor{Light: "#4F46E5", Dark: "#A5B4FC"} // headings
	cTeal   = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#2DD4BF"} // accent / links
	cGreen  = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"} // success
	cAmber  = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#F59E0B"} // warning
	cRed    = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"} // error
	cBlue   = lipgloss.AdaptiveColor{Light: "#1D4ED8", Dark: "#60A5FA"} // info
	cGray   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"} // muted / borders
)

// --- glyphs ------------------------------------------------------------------

// Glyph is the set of status and decoration glyphs. NewContext selects the
// Unicode set on a UTF-8 terminal and the ASCII set otherwise, so output stays
// legible in C/POSIX locales and dumb terminals.
type Glyph struct {
	OK      string // success
	Err     string // error / failure
	Warn    string // warning
	Info    string // informational
	Bullet  string // list item
	Arrow   string // pointer / transition
	Title   string // section heading marker
	Dot     string // active / running
	Pending string // not-yet-started
	TBranch string // tree: child with siblings below
	TLast   string // tree: last child
	TPipe   string // tree: vertical continuation
	TSpace  string // tree: blank continuation
}

var (
	glyphsUnicode = Glyph{
		OK: "✓", Err: "✗", Warn: "⚠", Info: "ℹ",
		Bullet: "•", Arrow: "→", Title: "▸", Dot: "●", Pending: "○",
		TBranch: "├─ ", TLast: "└─ ", TPipe: "│  ", TSpace: "   ",
	}
	glyphsASCII = Glyph{
		OK: "+", Err: "x", Warn: "!", Info: "i",
		Bullet: "*", Arrow: "->", Title: ">", Dot: "o", Pending: ".",
		TBranch: "|- ", TLast: "`- ", TPipe: "|  ", TSpace: "   ",
	}
)

// Glyphs returns the glyph set active for this invocation.
func (c *Context) Glyphs() Glyph { return c.gl }

// supportsUnicode reports whether the active locale looks UTF-8 capable. It
// honors the standard precedence (LC_ALL > LC_CTYPE > LANG) and assumes UTF-8
// when nothing is set (the modern default).
func supportsUnicode() bool {
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := os.Getenv(key)
		if v == "" {
			continue
		}
		u := strings.ToUpper(v)
		return strings.Contains(u, "UTF-8") || strings.Contains(u, "UTF8")
	}
	return true
}

// --- theme -------------------------------------------------------------------

type styled struct{ s lipgloss.Style }

func (x styled) render(color bool, s string) string {
	if !color {
		return s
	}
	return x.s.Render(s)
}

type theme struct {
	title    styled
	subtitle styled
	accent   styled
	faint    styled
	muted    styled
	ok       styled
	warn     styled
	err      styled
	info     styled
	hint     styled
	header   styled
	panel    styled
}

func newTheme() theme {
	s := lipgloss.NewStyle
	return theme{
		title:    styled{s().Bold(true).Foreground(cIndigo)},
		subtitle: styled{s().Bold(true).Foreground(cTeal)},
		accent:   styled{s().Foreground(cTeal)},
		faint:    styled{s().Faint(true)},
		muted:    styled{s().Foreground(cGray)},
		ok:       styled{s().Bold(true).Foreground(cGreen)},
		warn:     styled{s().Bold(true).Foreground(cAmber)},
		err:      styled{s().Bold(true).Foreground(cRed)},
		info:     styled{s().Bold(true).Foreground(cBlue)},
		hint:     styled{s().Faint(true).Italic(true)},
		header:   styled{s().Bold(true).Foreground(cIndigo)},
		panel:    styled{s().Border(lipgloss.RoundedBorder()).BorderForeground(cGray).Padding(0, 1)},
	}
}

// badge returns the fill style for an inline pill of the given kind. Fills
// carry their own background, so they read on any terminal background.
func (theme) badge(kind string) lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	white, black := lipgloss.Color("15"), lipgloss.Color("0")
	switch kind {
	case "ok":
		return base.Background(cGreen).Foreground(black)
	case "warn":
		return base.Background(cAmber).Foreground(black)
	case "err":
		return base.Background(cRed).Foreground(white)
	case "info":
		return base.Background(cBlue).Foreground(white)
	case "accent":
		return base.Background(cTeal).Foreground(black)
	default: // neutral
		return base.Background(cGray).Foreground(white)
	}
}

// faintMaybe dims s on a color TTY, leaving it untouched otherwise.
func (c *Context) faintMaybe(s string) string { return c.th.faint.render(c.Color, s) }

// --- inline text helpers -----------------------------------------------------

// Accent returns s styled as an accent (or unchanged when !Color).
func (c *Context) Accent(s string) string { return c.th.accent.render(c.Color, s) }

// Faint returns s styled faint/dim (or unchanged when !Color).
func (c *Context) Faint(s string) string { return c.th.faint.render(c.Color, s) }

// Muted returns s in the muted gray ink (or unchanged when !Color).
func (c *Context) Muted(s string) string { return c.th.muted.render(c.Color, s) }

// OK returns s styled as success (or unchanged when !Color).
func (c *Context) OK(s string) string { return c.th.ok.render(c.Color, s) }

// Link renders an OSC 8 hyperlink — clickable in supporting terminals — and
// falls back to "text (url)" when styling is off.
func (c *Context) Link(text, url string) string {
	if !c.Color {
		return text + " (" + url + ")"
	}
	return "\x1b]8;;" + url + "\x1b\\" + c.th.accent.s.Underline(true).Render(text) + "\x1b]8;;\x1b\\"
}

// Badge renders an inline pill label. kind is one of ok, warn, err, info,
// accent, or neutral. Off a color TTY it degrades to "[label]".
func (c *Context) Badge(kind, label string) string {
	if !c.Color {
		return "[" + label + "]"
	}
	return c.th.badge(kind).Render(label)
}

// --- status lines ------------------------------------------------------------

func (c *Context) status(glyph string, st styled, s string) string {
	if !c.Color {
		return glyph + " " + s
	}
	return st.s.Render(glyph) + " " + s
}

// Success returns a "✓ s" success line (glyph green on a color TTY).
func (c *Context) Success(s string) string { return c.status(c.gl.OK, c.th.ok, s) }

// Warning returns a "⚠ s" warning line.
func (c *Context) Warning(s string) string { return c.status(c.gl.Warn, c.th.warn, s) }

// Failure returns a "✗ s" failure line.
func (c *Context) Failure(s string) string { return c.status(c.gl.Err, c.th.err, s) }

// Info returns an "ℹ s" informational line.
func (c *Context) Info(s string) string { return c.status(c.gl.Info, c.th.info, s) }

// Severity returns an uppercased, glyph-prefixed, color-coded severity label
// (e.g. "✗ ERROR") for diagnostics and check output.
func (c *Context) Severity(s string) string {
	label := strings.ToUpper(s)
	switch s {
	case "error":
		return c.status(c.gl.Err, c.th.err, label)
	case "warning":
		return c.status(c.gl.Warn, c.th.warn, label)
	case "info":
		return c.status(c.gl.Info, c.th.info, label)
	default:
		return c.status(c.gl.Bullet, c.th.faint, label)
	}
}

// --- block primitives --------------------------------------------------------

// Title prints a styled section heading (e.g. "▸ repos").
func (c *Context) Title(s string) {
	fmt.Fprintln(c.Stdout, c.th.title.render(c.Color, c.gl.Title+" "+s))
}

// Subtitle prints a secondary heading in the accent ink.
func (c *Context) Subtitle(s string) {
	fmt.Fprintln(c.Stdout, c.th.subtitle.render(c.Color, s))
}

// List prints a bulleted list, one item per line, indented two spaces.
func (c *Context) List(items ...string) {
	bullet := c.gl.Bullet
	if c.Color {
		bullet = c.th.accent.s.Render(bullet)
	}
	for _, item := range items {
		fmt.Fprintf(c.Stdout, "  %s %s\n", bullet, item)
	}
}

// KV prints aligned key/value pairs — keys padded to a common width, in the
// muted ink, with values in the default foreground.
func (c *Context) KV(pairs ...[2]string) {
	width := 0
	for _, p := range pairs {
		if w := lipgloss.Width(p[0]); w > width {
			width = w
		}
	}
	for _, p := range pairs {
		pad := strings.Repeat(" ", width-lipgloss.Width(p[0]))
		fmt.Fprintf(c.Stdout, "%s%s  %s\n", c.Muted(p[0]), pad, p[1])
	}
}

// Rule prints a horizontal divider spanning the terminal width (capped at 80).
// A non-empty label is centered within the rule.
func (c *Context) Rule(label string) {
	width := c.ruleWidth()
	dash := "─"
	if !c.unicode {
		dash = "-"
	}
	if label == "" {
		fmt.Fprintln(c.Stdout, c.faintMaybe(strings.Repeat(dash, width)))
		return
	}
	mid := " " + label + " "
	side := (width - lipgloss.Width(mid)) / 2
	if side < 1 {
		side = 1
	}
	left := strings.Repeat(dash, side)
	right := strings.Repeat(dash, max(width-side-lipgloss.Width(mid), 1))
	fmt.Fprintln(c.Stdout, c.faintMaybe(left)+c.th.muted.render(c.Color, mid)+c.faintMaybe(right))
}

// Panel prints a rounded-border box with an optional bold title and body
// lines. Off a color TTY it falls back to a title line + indented body.
func (c *Context) Panel(title string, lines ...string) {
	if !c.Color {
		if title != "" {
			fmt.Fprintln(c.Stdout, title)
		}
		for _, ln := range lines {
			fmt.Fprintln(c.Stdout, "  "+ln)
		}
		return
	}
	body := strings.Join(lines, "\n")
	if title != "" {
		body = c.th.title.s.Render(title) + "\n" + body
	}
	fmt.Fprintln(c.Stdout, c.th.panel.s.Render(body))
}

// TreeNode is one node in a Tree: a label and zero or more children.
type TreeNode struct {
	Label    string
	Children []TreeNode
}

// Tree prints root's label, then its descendants as an indented tree using the
// active branch glyphs (├─ └─ │). Connectors are dimmed on a color TTY.
func (c *Context) Tree(root TreeNode) {
	fmt.Fprintln(c.Stdout, root.Label)
	c.treeChildren(root.Children, "")
}

func (c *Context) treeChildren(nodes []TreeNode, prefix string) {
	for i, n := range nodes {
		last := i == len(nodes)-1
		branch, cont := c.gl.TBranch, c.gl.TPipe
		if last {
			branch, cont = c.gl.TLast, c.gl.TSpace
		}
		fmt.Fprintln(c.Stdout, c.faintMaybe(prefix+branch)+n.Label)
		c.treeChildren(n.Children, prefix+cont)
	}
}

// Table renders a rounded-border, color-styled table on a TTY, or a clean
// tab-aligned table otherwise. (JSON mode never calls this — commands emit
// rows as data.)
func (c *Context) Table(headers []string, rows [][]string) {
	if c.Color {
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(c.th.muted.s).
			Headers(headers...).
			Rows(rows...).
			StyleFunc(func(row, _ int) lipgloss.Style {
				switch {
				case row == table.HeaderRow:
					return c.th.header.s.Padding(0, 1)
				case row%2 == 1:
					return c.th.faint.s.Padding(0, 1) // zebra striping
				default:
					return lipgloss.NewStyle().Padding(0, 1)
				}
			})
		fmt.Fprintln(c.Stdout, t.Render())
		return
	}
	tw := tabwriter.NewWriter(c.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	_ = tw.Flush()
}

// ruleWidth returns the terminal width for rules/dividers, capped at 80 and
// defaulting to 80 when the size can't be determined.
func (c *Context) ruleWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return min(w, 80)
}
