package main

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// A minimal RFC 5545 reader and writer. It keeps every property it doesn't
// understand byte-for-byte (after unfolding), so the output carries
// whatever Google sends: VTIMEZONEs, X- properties, alarms.

// Component is a BEGIN/END block, such as VCALENDAR or VEVENT.
type Component struct {
	Name     string
	Props    []Property
	Children []*Component
}

// Property is one unfolded content line. Params holds everything between
// the name and the value separator, including the leading ';', so a
// property that isn't edited is written back unchanged.
type Property struct {
	Name   string
	Params string
	Value  string
}

// Prop returns the first property with the given name, or nil.
func (c *Component) Prop(name string) *Property {
	for i := range c.Props {
		if c.Props[i].Name == name {
			return &c.Props[i]
		}
	}
	return nil
}

// Value returns the value of the first property with the given name.
func (c *Component) Value(name string) string {
	if p := c.Prop(name); p != nil {
		return p.Value
	}
	return ""
}

// RemoveProps drops every property for which drop returns true.
func (c *Component) RemoveProps(drop func(*Property) bool) {
	kept := c.Props[:0]
	for i := range c.Props {
		if !drop(&c.Props[i]) {
			kept = append(kept, c.Props[i])
		}
	}
	c.Props = kept
}

// Param returns the value of a parameter, unquoted, or "" if absent.
func (p *Property) Param(name string) string {
	for _, kv := range splitOutsideQuotes(strings.TrimPrefix(p.Params, ";"), ';') {
		k, v, ok := strings.Cut(kv, "=")
		if ok && strings.EqualFold(k, name) {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// Parse reads an iCalendar stream and returns its top-level component.
func Parse(data []byte) (*Component, error) {
	var stack []*Component
	var root *Component
	for i, line := range unfold(data) {
		if line == "" {
			continue
		}
		p, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		switch p.Name {
		case "BEGIN":
			c := &Component{Name: strings.ToUpper(p.Value)}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, c)
			} else if root == nil {
				root = c
			} else {
				return nil, errors.New("more than one top-level component")
			}
			stack = append(stack, c)
		case "END":
			if len(stack) == 0 || stack[len(stack)-1].Name != strings.ToUpper(p.Value) {
				return nil, fmt.Errorf("line %d: unexpected END:%s", i+1, p.Value)
			}
			stack = stack[:len(stack)-1]
		default:
			if len(stack) == 0 {
				return nil, fmt.Errorf("line %d: property outside a component", i+1)
			}
			c := stack[len(stack)-1]
			c.Props = append(c.Props, p)
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, errors.New("truncated calendar")
	}
	return root, nil
}

func unfold(data []byte) []string {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if len(l) > 0 && (l[0] == ' ' || l[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

func parseLine(line string) (Property, error) {
	nameEnd := strings.IndexAny(line, ";:")
	if nameEnd <= 0 {
		return Property{}, errors.New("no property name")
	}
	p := Property{Name: strings.ToUpper(line[:nameEnd])}
	// The value starts at the first ':' outside a quoted parameter value.
	inQuote := false
	for i := nameEnd; i < len(line); i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case ':':
			if !inQuote {
				p.Params = line[nameEnd:i]
				p.Value = line[i+1:]
				return p, nil
			}
		}
	}
	return Property{}, errors.New("no value")
}

func splitOutsideQuotes(s string, sep byte) []string {
	var parts []string
	inQuote, start := false, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuote = !inQuote
		case sep:
			if !inQuote {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// Serialize writes c with CRLF line endings, folded at 75 octets.
func (c *Component) Serialize() []byte {
	var b bytes.Buffer
	c.writeTo(&b)
	return b.Bytes()
}

func (c *Component) writeTo(b *bytes.Buffer) {
	writeFolded(b, "BEGIN:"+c.Name)
	for _, p := range c.Props {
		writeFolded(b, p.Name+p.Params+":"+p.Value)
	}
	for _, child := range c.Children {
		child.writeTo(b)
	}
	writeFolded(b, "END:"+c.Name)
}

func writeFolded(b *bytes.Buffer, line string) {
	limit := 75
	for len(line) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n ")
		line = line[cut:]
		limit = 74 // continuation lines spend one octet on the leading space
	}
	b.WriteString(line)
	b.WriteString("\r\n")
}

// Time is a parsed DATE or DATE-TIME value.
type Time struct {
	time.Time
	AllDay bool
}

// ParseTime reads a DTSTART-like property. TZIDs Go doesn't know fall back
// to UTC: an hour's error matters less than dropping the event.
func ParseTime(p *Property) (Time, error) {
	v := p.Value
	if strings.EqualFold(p.Param("VALUE"), "DATE") || len(v) == 8 {
		t, err := time.ParseInLocation("20060102", v, time.UTC)
		return Time{t, true}, err
	}
	if strings.HasSuffix(v, "Z") {
		t, err := time.Parse("20060102T150405Z", v)
		return Time{Time: t}, err
	}
	loc := time.UTC
	if tzid := p.Param("TZID"); tzid != "" {
		if l, err := time.LoadLocation(tzid); err == nil {
			loc = l
		}
	}
	t, err := time.ParseInLocation("20060102T150405", v, loc)
	return Time{Time: t}, err
}

// ParseDuration reads an RFC 5545 DURATION, such as PT1H30M or P2D.
func ParseDuration(s string) (time.Duration, error) {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimLeft(s, "+-")
	if !strings.HasPrefix(s, "P") {
		return 0, fmt.Errorf("bad duration %q", s)
	}
	s = s[1:]
	var d time.Duration
	inTime := false
	num := ""
	for _, r := range s {
		switch {
		case r == 'T':
			inTime = true
		case r >= '0' && r <= '9':
			num += string(r)
		default:
			n, err := strconv.Atoi(num)
			if err != nil {
				return 0, fmt.Errorf("bad duration %q", s)
			}
			num = ""
			unit := map[rune]time.Duration{'W': 7 * 24 * time.Hour, 'D': 24 * time.Hour}
			if inTime {
				unit = map[rune]time.Duration{'H': time.Hour, 'M': time.Minute, 'S': time.Second}
			}
			u, ok := unit[r]
			if !ok {
				return 0, fmt.Errorf("bad duration %q", s)
			}
			d += time.Duration(n) * u
		}
	}
	if neg {
		d = -d
	}
	return d, nil
}
