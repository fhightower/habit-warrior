// Command hw tracks daily habits from the terminal.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/fhightower/habit-warrior/internal/hdate"
	"github.com/fhightower/habit-warrior/internal/model"
	"github.com/fhightower/habit-warrior/internal/render"
	"github.com/fhightower/habit-warrior/internal/storage"
)

// version is the build version reported by --version.
const version = "0.1.0"

const usage = `hw - habit warrior, a tracker for daily habits

Usage:
  hw add <name> [+tag...]          Start tracking a habit
  hw done <habit|+tag> [date]      Mark done (default: today)
  hw undo <habit|+tag> [date]      Unmark
  hw list [+tag] [-tag] [--all]    Table of habits, streaks, last 30 days
  hw cal [habit] [+tag] [--weeks N]  Heatmap (default: 26 weeks)
  hw stats [habit] [+tag]          Streaks and completion rates
  hw rename <habit> <new name>     Rename
  hw tag <habit> +add -remove      Change tags
  hw archive <habit>               Hide without deleting
  hw unarchive <habit>             Bring back
  hw delete <habit> --force        Delete, completions and all

Habits are selected by ID, exact name, unique name prefix, or unique substring.
Dates accept: today, yesterday, 3d, 3 days ago, mon, 2026-08-01.

Flags:
  --json          Machine-readable output
  --data <path>   Data file (default $HABIT_WARRIOR_DATA or
                  $XDG_DATA_HOME/habit-warrior/habits.json)
  --no-color      Disable color
  --version       Print version
  --help          Print this help
`

// exitError carries a message and the status code to exit with: 1 for user
// error, 2 for anything to do with reading or writing the data file.
type exitError struct {
	code int
	msg  string
}

func (e exitError) Error() string { return e.msg }

func userErr(format string, a ...any) error {
	return exitError{code: 1, msg: fmt.Sprintf(format, a...)}
}

func ioErr(err error) error {
	return exitError{code: 2, msg: err.Error()}
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "hw: "+err.Error())
		code := 1
		var ee exitError
		if errorsAs(err, &ee) {
			code = ee.code
		}
		os.Exit(code)
	}
}

// errorsAs is errors.As specialized to exitError, kept local to avoid an
// import for one call.
func errorsAs(err error, target *exitError) bool {
	if ee, ok := err.(exitError); ok {
		*target = ee
		return true
	}
	return false
}

// options are the flags that apply to every subcommand.
type options struct {
	jsonOut bool
	path    string
	color   bool
}

func run(argv []string, stdout io.Writer) error {
	var (
		opts     options
		noColor  bool
		showHelp bool
		showVer  bool
		args     []string
	)

	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--json":
			opts.jsonOut = true
		case a == "--no-color":
			noColor = true
		case a == "--help" || a == "-h" || a == "help":
			showHelp = true
		case a == "--version" || a == "-v":
			showVer = true
		case a == "--data":
			if i+1 >= len(argv) {
				return userErr("--data needs a path")
			}
			i++
			opts.path = argv[i]
		case strings.HasPrefix(a, "--data="):
			opts.path = strings.TrimPrefix(a, "--data=")
		default:
			args = append(args, a)
		}
	}

	if showVer {
		fmt.Fprintf(stdout, "hw %s\n", version)
		return nil
	}
	if showHelp || len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return nil
	}

	opts.color = !noColor && os.Getenv("NO_COLOR") == "" && isTerminal(stdout)

	if opts.path == "" {
		p, err := storage.DefaultPath()
		if err != nil {
			return ioErr(err)
		}
		opts.path = p
	}

	store, err := storage.Load(opts.path)
	if err != nil {
		return ioErr(err)
	}

	cmd, rest := args[0], args[1:]
	today := hdate.Today()

	switch cmd {
	case "add":
		return cmdAdd(stdout, store, opts, rest, today)
	case "done":
		return cmdMark(stdout, store, opts, rest, today, true)
	case "undo":
		return cmdMark(stdout, store, opts, rest, today, false)
	case "list", "ls":
		return cmdList(stdout, store, opts, rest, today)
	case "cal", "calendar":
		return cmdCal(stdout, store, opts, rest, today)
	case "stats", "stat":
		return cmdStats(stdout, store, opts, rest, today)
	case "rename":
		return cmdRename(stdout, store, opts, rest, today)
	case "tag":
		return cmdTag(stdout, store, opts, rest, today)
	case "archive":
		return cmdArchive(stdout, store, opts, rest, today, true)
	case "unarchive":
		return cmdArchive(stdout, store, opts, rest, today, false)
	case "delete", "del", "rm":
		return cmdDelete(stdout, store, opts, rest)
	default:
		return userErr("unknown command %q (try: hw --help)", cmd)
	}
}

// isTerminal reports whether output is going to a terminal, so color is only
// emitted when a human is looking. Anything that is not an *os.File (a test
// buffer, say) counts as not a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// takeFlag removes a boolean flag from args, reporting whether it was there.
func takeFlag(args []string, name string) ([]string, bool) {
	out := args[:0:0]
	found := false
	for _, a := range args {
		if a == name {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

// takeValue removes a "--name value" or "--name=value" pair from args.
func takeValue(args []string, name string) ([]string, string, error) {
	out := args[:0:0]
	val := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == name:
			if i+1 >= len(args) {
				return nil, "", userErr("%s needs a value", name)
			}
			i++
			val = args[i]
		case strings.HasPrefix(a, name+"="):
			val = strings.TrimPrefix(a, name+"=")
		default:
			out = append(out, a)
		}
	}
	return out, val, nil
}

// save writes the store, mapping any failure to an I/O exit code.
func save(opts options, store *model.Store) error {
	if err := storage.Save(opts.path, store); err != nil {
		return ioErr(err)
	}
	return nil
}

// emit prints affected habits as JSON, or a plain sentence.
func emit(stdout io.Writer, opts options, habits []*model.Habit, today hdate.Date, msg string) error {
	if opts.jsonOut {
		return writeJSON(stdout, render.BuildRows(habits, today))
	}
	fmt.Fprintln(stdout, msg)
	return nil
}

func writeJSON(stdout io.Writer, v any) error {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return ioErr(err)
	}
	return nil
}

func cmdAdd(stdout io.Writer, store *model.Store, opts options, args []string, today hdate.Date) error {
	filter, rest := model.ParseArgs(args)
	name := strings.Join(rest, " ")
	if name == "" {
		return userErr("usage: hw add <name> [+tag...]")
	}
	if len(filter.Exclude) > 0 {
		return userErr("cannot use -tag when adding a habit")
	}
	h, err := store.Add(name, filter.Include, today)
	if err != nil {
		return userErr("%s", err)
	}
	if err := save(opts, store); err != nil {
		return err
	}
	return emit(stdout, opts, []*model.Habit{h}, today,
		fmt.Sprintf("Added %s (#%d)", h.Name, h.ID))
}

// cmdMark handles both done and undo, for one habit or every habit matching a
// tag filter.
func cmdMark(stdout io.Writer, store *model.Store, opts options, args []string, today hdate.Date, done bool) error {
	verb := "done"
	if !done {
		verb = "undo"
	}

	filter, rest := model.ParseArgs(args)

	var targets []*model.Habit
	if filter.Empty() {
		if len(rest) == 0 {
			return userErr("usage: hw %s <habit|+tag> [date]", verb)
		}
		h, err := store.Find(rest[0])
		if err != nil {
			return userErr("%s", err)
		}
		targets = []*model.Habit{h}
		rest = rest[1:]
	} else {
		targets = store.Select(filter, false)
		if len(targets) == 0 {
			return userErr("no habits match that filter")
		}
	}

	day := today
	if expr := strings.Join(rest, " "); expr != "" {
		d, err := hdate.Parse(expr, today)
		if err != nil {
			// In bulk mode everything after the tags has to be a date, so a
			// leftover habit name lands here rather than being ignored.
			if !filter.Empty() {
				return userErr("give a habit or tag filters, not both")
			}
			return userErr("%s", err)
		}
		day = d
	}
	if day.After(today) {
		return userErr("cannot log %s: that day has not happened yet", day)
	}

	changed := 0
	for _, h := range targets {
		if done {
			if h.MarkDone(day) {
				changed++
			}
		} else if h.Undo(day) {
			changed++
		}
	}

	if changed > 0 {
		if err := save(opts, store); err != nil {
			return err
		}
	}

	when := "today"
	if !day.Equal(today) {
		when = day.String()
	}
	names := make([]string, len(targets))
	for i, h := range targets {
		names[i] = h.Name
	}
	action := "Marked done"
	if !done {
		action = "Unmarked"
	}
	msg := fmt.Sprintf("%s for %s: %s", action, when, strings.Join(names, ", "))
	if changed == 0 {
		msg = fmt.Sprintf("No change for %s: %s", when, strings.Join(names, ", "))
	}
	return emit(stdout, opts, targets, today, msg)
}

func cmdList(stdout io.Writer, store *model.Store, opts options, args []string, today hdate.Date) error {
	args, all := takeFlag(args, "--all")
	filter, rest := model.ParseArgs(args)
	if len(rest) > 0 {
		return userErr("list takes only tag filters, got %q", rest[0])
	}

	habits := store.Select(filter, all)
	rows := render.BuildRows(habits, today)
	if opts.jsonOut {
		return writeJSON(stdout, rows)
	}
	render.List(stdout, rows, opts.color)
	return nil
}

func cmdCal(stdout io.Writer, store *model.Store, opts options, args []string, today hdate.Date) error {
	args, weeksArg, err := takeValue(args, "--weeks")
	if err != nil {
		return err
	}
	weeks := 26
	if weeksArg != "" {
		n, err := strconv.Atoi(weeksArg)
		if err != nil || n < 1 {
			return userErr("--weeks needs a positive number, got %q", weeksArg)
		}
		weeks = n
	}

	filter, rest := model.ParseArgs(args)

	var habits []*model.Habit
	title := "All habits"
	switch {
	case len(rest) > 0:
		if !filter.Empty() {
			return userErr("give a habit or tag filters, not both")
		}
		h, err := store.Find(strings.Join(rest, " "))
		if err != nil {
			return userErr("%s", err)
		}
		habits = []*model.Habit{h}
		title = h.Name
	default:
		habits = store.Select(filter, false)
		if !filter.Empty() {
			title = "Habits matching " + strings.Join(filter.Include, ", ")
		}
	}
	if len(habits) == 0 {
		return userErr("no habits to show")
	}

	cal := render.BuildCal(habits, today, weeks)
	if opts.jsonOut {
		return writeJSON(stdout, cal)
	}
	render.Cal(stdout, cal, title, opts.color)
	return nil
}

func cmdStats(stdout io.Writer, store *model.Store, opts options, args []string, today hdate.Date) error {
	args, all := takeFlag(args, "--all")
	filter, rest := model.ParseArgs(args)

	var habits []*model.Habit
	if len(rest) > 0 {
		if !filter.Empty() {
			return userErr("give a habit or tag filters, not both")
		}
		h, err := store.Find(strings.Join(rest, " "))
		if err != nil {
			return userErr("%s", err)
		}
		habits = []*model.Habit{h}
	} else {
		habits = store.Select(filter, all)
	}
	if len(habits) == 0 {
		return userErr("no habits to show")
	}

	if opts.jsonOut {
		out := make([]model.Stats, 0, len(habits))
		for _, h := range habits {
			out = append(out, h.Stats(today))
		}
		if len(out) == 1 {
			return writeJSON(stdout, out[0])
		}
		return writeJSON(stdout, out)
	}

	for i, h := range habits {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		render.Stats(stdout, h.Stats(today), opts.color)
	}
	return nil
}

func cmdRename(stdout io.Writer, store *model.Store, opts options, args []string, today hdate.Date) error {
	if len(args) < 2 {
		return userErr("usage: hw rename <habit> <new name>")
	}
	h, err := store.Find(args[0])
	if err != nil {
		return userErr("%s", err)
	}
	newName := strings.TrimSpace(strings.Join(args[1:], " "))
	if newName == "" {
		return userErr("new name cannot be empty")
	}
	if _, err := strconv.Atoi(newName); err == nil {
		return userErr("habit name cannot be a number: it would collide with an ID")
	}
	for i := range store.Habits {
		if store.Habits[i].ID != h.ID && strings.EqualFold(store.Habits[i].Name, newName) {
			return userErr("habit %q already exists", store.Habits[i].Name)
		}
	}
	old := h.Name
	h.Name = newName
	if err := save(opts, store); err != nil {
		return err
	}
	return emit(stdout, opts, []*model.Habit{h}, today,
		fmt.Sprintf("Renamed %s to %s", old, h.Name))
}

func cmdTag(stdout io.Writer, store *model.Store, opts options, args []string, today hdate.Date) error {
	filter, rest := model.ParseArgs(args)
	if len(rest) == 0 {
		return userErr("usage: hw tag <habit> +add -remove")
	}
	if filter.Empty() {
		return userErr("give at least one +tag to add or -tag to remove")
	}
	h, err := store.Find(strings.Join(rest, " "))
	if err != nil {
		return userErr("%s", err)
	}
	changed := false
	for _, t := range filter.Include {
		changed = h.AddTag(t) || changed
	}
	for _, t := range filter.Exclude {
		changed = h.RemoveTag(t) || changed
	}
	if changed {
		if err := save(opts, store); err != nil {
			return err
		}
	}
	tags := "(none)"
	if len(h.Tags) > 0 {
		tags = "+" + strings.Join(h.Tags, " +")
	}
	return emit(stdout, opts, []*model.Habit{h}, today,
		fmt.Sprintf("%s tags: %s", h.Name, tags))
}

func cmdArchive(stdout io.Writer, store *model.Store, opts options, args []string, today hdate.Date, archive bool) error {
	if len(args) == 0 {
		verb := "archive"
		if !archive {
			verb = "unarchive"
		}
		return userErr("usage: hw %s <habit>", verb)
	}
	h, err := store.Find(strings.Join(args, " "))
	if err != nil {
		return userErr("%s", err)
	}
	if h.Archived == archive {
		state := "already archived"
		if !archive {
			state = "not archived"
		}
		return userErr("%s is %s", h.Name, state)
	}
	h.Archived = archive
	if err := save(opts, store); err != nil {
		return err
	}
	msg := fmt.Sprintf("Archived %s (history kept; see it with hw list --all)", h.Name)
	if !archive {
		msg = fmt.Sprintf("Restored %s", h.Name)
	}
	return emit(stdout, opts, []*model.Habit{h}, today, msg)
}

func cmdDelete(stdout io.Writer, store *model.Store, opts options, args []string) error {
	args, force := takeFlag(args, "--force")
	if len(args) == 0 {
		return userErr("usage: hw delete <habit> --force")
	}
	h, err := store.Find(strings.Join(args, " "))
	if err != nil {
		return userErr("%s", err)
	}
	if !force {
		return userErr("deleting %s discards %d logged days; pass --force to confirm (or hw archive %s to keep the history)",
			h.Name, len(h.Done), h.Name)
	}
	name, count := h.Name, len(h.Done)
	store.Delete(h.ID)
	if err := save(opts, store); err != nil {
		return err
	}
	if opts.jsonOut {
		return writeJSON(stdout, map[string]any{"deleted": name, "days_discarded": count})
	}
	fmt.Fprintf(stdout, "Deleted %s and %d logged days\n", name, count)
	return nil
}
