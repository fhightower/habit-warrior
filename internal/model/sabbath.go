package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/fhightower/habit-warrior/internal/hdate"
)

// DefaultSabbath is the rest day used when nothing has been configured.
const DefaultSabbath = time.Sunday

// Sabbath is the weekly rest day. A rest day neither breaks a streak nor
// counts against a completion rate: it is skipped, not failed. The zero value
// rests never, so anything holding one by value behaves as if sabbath mode
// were off.
type Sabbath struct {
	Day time.Weekday
	On  bool
}

// Rest reports whether d is a rest day.
func (s Sabbath) Rest(d hdate.Date) bool {
	return s.On && d.Weekday() == s.Day
}

// String names the rest day, or reports that there is none.
func (s Sabbath) String() string {
	if !s.On {
		return "off"
	}
	return s.Day.String()
}

// sabbathName is the rest day for reports, empty when sabbath mode is off, so
// a JSON consumer can test the field rather than compare it to "off".
func sabbathName(s Sabbath) string {
	if !s.On {
		return ""
	}
	return s.Day.String()
}

// Config holds store-wide settings. Every field is optional: an absent field
// means the default, which keeps hand-edited and older files valid.
type Config struct {
	// Sabbath is the rest day as a lowercase weekday name, or "none" when
	// disabled. Nil or empty means the default, which is on at Sunday.
	Sabbath *string `json:"sabbath,omitempty"`
}

// offValues are the spellings that disable a day setting. "none" is what hw
// writes; the rest are accepted so a hand-edited file does the obvious thing.
var offValues = map[string]bool{"none": true, "off": true, "never": true, "no": true, "disabled": true}

// Sabbath resolves the configured rest day. An unset, empty or unparsable
// value falls back to the default rather than failing, so a hand-edited file
// still reports rather than refusing to run.
func (s *Store) Sabbath() Sabbath {
	def := Sabbath{Day: DefaultSabbath, On: true}
	if s.Config == nil || s.Config.Sabbath == nil {
		return def
	}
	v := strings.ToLower(strings.TrimSpace(*s.Config.Sabbath))
	if v == "" {
		return def
	}
	if offValues[v] {
		return Sabbath{}
	}
	if wd, ok := hdate.ParseWeekday(v); ok {
		return Sabbath{Day: wd, On: true}
	}
	return def
}

// SetSabbath validates and stores a rest day. It accepts any weekday name or
// abbreviation, "on" for the default day, and off/none/never to disable.
func (s *Store) SetSabbath(v string) error {
	norm := strings.ToLower(strings.TrimSpace(v))
	var canonical string
	switch {
	case norm == "":
		return fmt.Errorf("give a weekday, or off to disable")
	case norm == "on":
		canonical = strings.ToLower(DefaultSabbath.String())
	case offValues[norm]:
		canonical = "none"
	default:
		wd, ok := hdate.ParseWeekday(norm)
		if !ok {
			return fmt.Errorf("%q is not a weekday: give a day like sunday, on, or off", v)
		}
		canonical = strings.ToLower(wd.String())
	}
	if s.Config == nil {
		s.Config = &Config{}
	}
	s.Config.Sabbath = &canonical
	return nil
}
