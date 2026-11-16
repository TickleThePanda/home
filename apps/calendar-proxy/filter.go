package main

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

// Options controls one Filter run.
type Options struct {
	Now      time.Time
	PastDays int
	MaxBytes int
	// Self is the calendar owner's email. Events where this attendee has
	// declined are dropped.
	Self string
}

// Stats describes what a Filter run kept and dropped, for logs and metrics.
type Stats struct {
	InBytes  int
	OutBytes int
	Kept     int
	// Dropped counts VEVENTs by reason: cancelled, declined, old, size.
	Dropped map[string]int
	// PastDays is the window actually used, after any shrinking for size.
	PastDays int
	// Degraded lists the size steps applied: description, past-window,
	// short-events. Empty when the feed fitted without them.
	Degraded []string
}

const (
	descriptionLimit = 1000
	// Events at least this long, all-day or multi-day are never dropped
	// for size: they're the travel and long blocks the feed exists for.
	shortEvent = 4 * time.Hour
)

// Filter returns a smaller copy of an iCalendar feed. In order:
//
//  1. Drop cancelled and declined events. A cancelled or declined instance
//     of a series becomes an EXDATE on the series, so the rest of the
//     series renders as before.
//  2. Drop standalone events and whole series that ended before the cutoff
//     (now - PastDays), and series overrides entirely before it (again
//     leaving an EXDATE).
//  3. Strip ATTENDEE and X-ALT-DESC, and truncate DESCRIPTION.
//
// While the result exceeds MaxBytes, it then drops DESCRIPTION, halves the
// past window down to zero, and finally drops the furthest-future short
// standalone events.
func Filter(src []byte, o Options) ([]byte, Stats, error) {
	st := Stats{InBytes: len(src), PastDays: o.PastDays}
	o.Self = strings.ToLower(o.Self)

	noDesc := false
	var cal *Component
	var res result
	for {
		root, err := Parse(src)
		if err != nil {
			return nil, st, err
		}
		if root.Name != "VCALENDAR" {
			return nil, st, errors.New("not a VCALENDAR")
		}
		res = build(root, o, st.PastDays, noDesc)
		cal = root
		size := len(cal.Serialize())
		switch {
		case size <= o.MaxBytes:
		case !noDesc:
			noDesc = true
			st.Degraded = append(st.Degraded, "description")
			continue
		case st.PastDays > 0:
			if !slices.Contains(st.Degraded, "past-window") {
				st.Degraded = append(st.Degraded, "past-window")
			}
			st.PastDays /= 2
			continue
		default:
			dropped := dropShortFutureEvents(cal, res.standalone, o.Now, size-o.MaxBytes)
			if dropped > 0 {
				st.Degraded = append(st.Degraded, "short-events")
				res.dropped["size"] += dropped
			}
		}
		break
	}

	out := cal.Serialize()
	st.OutBytes = len(out)
	st.Dropped = res.dropped
	for _, c := range cal.Children {
		if c.Name == "VEVENT" {
			st.Kept++
		}
	}
	return out, st, nil
}

type result struct {
	dropped map[string]int
	// standalone holds the kept events that aren't part of a series: the
	// candidates for dropping on size.
	standalone []*Component
}

// build filters cal's VEVENTs in place.
func build(cal *Component, o Options, pastDays int, noDesc bool) result {
	cutoff := o.Now.AddDate(0, 0, -pastDays)
	res := result{dropped: map[string]int{}}

	masters := map[string]*Component{}
	overrides := map[string][]*Component{}
	for _, c := range cal.Children {
		if c.Name != "VEVENT" {
			continue
		}
		uid := c.Value("UID")
		switch {
		case c.Prop("RECURRENCE-ID") != nil:
			overrides[uid] = append(overrides[uid], c)
		case c.Prop("RRULE") != nil || c.Prop("RDATE") != nil:
			masters[uid] = c
		}
	}

	drop := map[*Component]bool{}
	dropAs := func(c *Component, reason string) {
		drop[c] = true
		res.dropped[reason]++
	}

	for _, c := range cal.Children {
		if c.Name != "VEVENT" {
			continue
		}
		uid := c.Value("UID")
		isOverride := c.Prop("RECURRENCE-ID") != nil
		if c == masters[uid] || (isOverride && masters[uid] != nil) {
			continue // handled with its series below
		}
		// A standalone event, or an override whose series isn't in this
		// feed (an invite to one instance of someone else's series).
		switch {
		case isCancelled(c):
			dropAs(c, "cancelled")
		case isDeclined(c, o.Self):
			dropAs(c, "declined")
		default:
			if _, end, err := span(c); err == nil && end.Before(cutoff) {
				dropAs(c, "old")
			}
		}
	}

	for uid, m := range masters {
		ovs := overrides[uid]
		if r := unwanted(m, o.Self); r != "" {
			dropAs(m, r)
			for _, ov := range ovs {
				dropAs(ov, r)
			}
			continue
		}
		var live []*Component
		for _, ov := range ovs {
			if r := unwanted(ov, o.Self); r != "" && exclude(m, ov) {
				dropAs(ov, r)
				continue
			}
			live = append(live, ov)
		}
		if seriesEnded(m, live, cutoff) {
			dropAs(m, "old")
			for _, ov := range live {
				dropAs(ov, "old")
			}
			continue
		}
		for _, ov := range live {
			_, end, err := span(ov)
			if err != nil || !end.Before(cutoff) {
				continue
			}
			rid, err := ParseTime(ov.Prop("RECURRENCE-ID"))
			if err == nil && rid.Before(cutoff) && exclude(m, ov) {
				dropAs(ov, "old")
			}
		}
	}

	usedTZ := map[string]bool{}
	kept := cal.Children[:0]
	for _, c := range cal.Children {
		if drop[c] {
			continue
		}
		if c.Name == "VEVENT" {
			strip(c, noDesc)
			for i := range c.Props {
				if tz := c.Props[i].Param("TZID"); tz != "" {
					usedTZ[tz] = true
				}
			}
			isOverride := c.Prop("RECURRENCE-ID") != nil
			if c != masters[c.Value("UID")] && !(isOverride && masters[c.Value("UID")] != nil) {
				res.standalone = append(res.standalone, c)
			}
		}
		kept = append(kept, c)
	}
	// Drop VTIMEZONEs no remaining event refers to.
	cal.Children = kept[:0]
	for _, c := range kept {
		if c.Name == "VTIMEZONE" && !usedTZ[c.Value("TZID")] {
			continue
		}
		cal.Children = append(cal.Children, c)
	}
	return res
}

func unwanted(c *Component, self string) string {
	switch {
	case isCancelled(c):
		return "cancelled"
	case isDeclined(c, self):
		return "declined"
	}
	return ""
}

func isCancelled(c *Component) bool {
	return strings.EqualFold(c.Value("STATUS"), "CANCELLED")
}

func isDeclined(c *Component, self string) bool {
	if self == "" {
		return false
	}
	for i := range c.Props {
		p := &c.Props[i]
		if p.Name != "ATTENDEE" {
			continue
		}
		email := strings.ToLower(strings.TrimPrefix(strings.ToLower(p.Value), "mailto:"))
		if email == self && strings.EqualFold(p.Param("PARTSTAT"), "DECLINED") {
			return true
		}
	}
	return false
}

// exclude adds ov's RECURRENCE-ID to m as an EXDATE, so m doesn't
// regenerate the instance once ov is dropped. It returns false, leaving
// ov in place, for a RANGE=THISANDFUTURE override, which an EXDATE can't
// stand in for.
func exclude(m, ov *Component) bool {
	rid := ov.Prop("RECURRENCE-ID")
	if rid.Param("RANGE") != "" {
		return false
	}
	m.Props = append(m.Props, Property{Name: "EXDATE", Params: rid.Params, Value: rid.Value})
	return true
}

// span returns an event's start and end. Without DTEND or DURATION, an
// all-day event lasts a day and a timed one is instantaneous (RFC 5545).
func span(c *Component) (time.Time, time.Time, error) {
	dtstart := c.Prop("DTSTART")
	if dtstart == nil {
		return time.Time{}, time.Time{}, errors.New("no DTSTART")
	}
	start, err := ParseTime(dtstart)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if p := c.Prop("DTEND"); p != nil {
		end, err := ParseTime(p)
		return start.Time, end.Time, err
	}
	if d := c.Value("DURATION"); d != "" {
		dur, err := ParseDuration(d)
		return start.Time, start.Add(dur), err
	}
	if start.AllDay {
		return start.Time, start.AddDate(0, 0, 1), nil
	}
	return start.Time, start.Time, nil
}

func isAllDay(c *Component) bool {
	p := c.Prop("DTSTART")
	if p == nil {
		return false
	}
	t, err := ParseTime(p)
	return err == nil && t.AllDay
}

// seriesEnded reports whether neither the series nor any of its overrides
// has an instance ending after cutoff. A series it can't parse counts as
// live: better too big than missing meetings.
func seriesEnded(m *Component, overrides []*Component, cutoff time.Time) bool {
	for _, ov := range overrides {
		if _, end, err := span(ov); err != nil || !end.Before(cutoff) {
			return false
		}
	}
	start, end, err := span(m)
	if err != nil {
		return false
	}
	set, err := recurrence(m)
	if err != nil {
		return false
	}
	// An instance starting after cutoff-duration ends after cutoff.
	next := set.After(cutoff.Add(-end.Sub(start)), false)
	return next.IsZero()
}

func recurrence(m *Component) (*rrule.Set, error) {
	dtstart, err := ParseTime(m.Prop("DTSTART"))
	if err != nil {
		return nil, err
	}
	set := &rrule.Set{}
	set.DTStart(dtstart.Time)
	for i := range m.Props {
		p := &m.Props[i]
		switch p.Name {
		case "RRULE":
			opt, err := rrule.StrToROptionInLocation(p.Value, dtstart.Location())
			if err != nil {
				return nil, fmt.Errorf("RRULE: %w", err)
			}
			opt.Dtstart = dtstart.Time
			r, err := rrule.NewRRule(*opt)
			if err != nil {
				return nil, fmt.Errorf("RRULE: %w", err)
			}
			set.RRule(r)
		case "RDATE", "EXDATE":
			for _, v := range strings.Split(p.Value, ",") {
				t, err := ParseTime(&Property{Params: p.Params, Value: v})
				if err != nil {
					continue // RDATE;VALUE=PERIOD; rare, and safe to skip
				}
				if p.Name == "RDATE" {
					set.RDate(t.Time)
				} else {
					set.ExDate(t.Time)
				}
			}
		}
	}
	return set, nil
}

func strip(c *Component, noDesc bool) {
	c.RemoveProps(func(p *Property) bool {
		return p.Name == "ATTENDEE" || p.Name == "X-ALT-DESC" || (noDesc && p.Name == "DESCRIPTION")
	})
	if p := c.Prop("DESCRIPTION"); p != nil && len(p.Value) > descriptionLimit {
		p.Value = truncateText(p.Value, descriptionLimit) + "…"
	}
}

// truncateText cuts an escaped TEXT value to at most n bytes without
// splitting a UTF-8 rune or a backslash escape.
func truncateText(v string, n int) string {
	for n > 0 && n < len(v) && v[n]&0xC0 == 0x80 {
		n--
	}
	v = v[:n]
	trailing := len(v) - len(strings.TrimRight(v, `\`))
	if trailing%2 == 1 {
		v = v[:len(v)-1]
	}
	return v
}

// dropShortFutureEvents removes short standalone events from cal, furthest
// future first, until at least excess bytes are freed. It returns how many
// it removed.
func dropShortFutureEvents(cal *Component, standalone []*Component, now time.Time, excess int) int {
	type cand struct {
		c     *Component
		start time.Time
	}
	var cands []cand
	for _, c := range standalone {
		start, end, err := span(c)
		if err != nil || isAllDay(c) || !start.After(now) || end.Sub(start) >= shortEvent {
			continue
		}
		cands = append(cands, cand{c, start})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].start.After(cands[j].start) })

	drop := map[*Component]bool{}
	for _, cd := range cands {
		if excess <= 0 {
			break
		}
		drop[cd.c] = true
		excess -= len(cd.c.Serialize())
	}
	kept := cal.Children[:0]
	for _, c := range cal.Children {
		if !drop[c] {
			kept = append(kept, c)
		}
	}
	cal.Children = kept
	return len(drop)
}
