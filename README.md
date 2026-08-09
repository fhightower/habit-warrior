# habit-warrior

A taskwarrior-flavored CLI for daily habits. One binary, `hw`, no dependencies
outside the Go standard library, and a plain JSON data file you can read, diff,
and sync yourself.

Habits are binary and daily: on any given day you did it or you didn't.

## Install

```
go install github.com/fhightower/habit-warrior/cmd/hw@latest
```

From a clone, `go install ./cmd/hw` does the same thing. Either way the binary
lands in `$(go env GOPATH)/bin` — add that to your `PATH` if `hw` comes back as
an unknown command:

```fish
fish_add_path (go env GOPATH)/bin        # fish
export PATH="$(go env GOPATH)/bin:$PATH" # bash, zsh
```

To build without installing, `go build -o hw ./cmd/hw` writes `hw` into the
current directory, where you run it as `./hw`.

## Use

```
hw add workout +health +morning
hw add "read a book" +mind

hw done workout              # today
hw done workout yesterday    # backfill
hw done read 3 days ago
hw done +morning              # everything tagged morning

hw list
hw list +health -evening
hw cal --weeks 12
hw stats workout
```

```
 ID  Habit        Tags            Today  Streak  Last 30
  1  workout      health,morning  ✓          12    26/30
  2  read a book  mind            ·           0    10/30
```

A `!` after a streak means it survives only if you do the habit today: the
count is still running from yesterday.

### Selecting habits

Anything that names one habit works, tried in this order: ID, exact name,
unique name prefix, unique substring. Ambiguity is an error listing the
candidates, never a guess.

```
hw done 1
hw done workout
hw done work
hw done out
```

### Dates

`today`, `yesterday`, `tomorrow`, `3d`, `3 days ago`, `mon` (the most recent
Monday), or an ISO date like `2026-08-01`. Future days are refused.

### Tags

`+tag` requires a tag, `-tag` excludes one, and several combine with AND.
Filters work on `list`, `cal`, `stats`, and on `done`/`undo` for marking a
whole group at once.

### Commands

| Command | What it does |
|---|---|
| `hw add <name> [+tag...]` | Start tracking a habit |
| `hw done <habit\|+tag> [date]` | Mark done |
| `hw undo <habit\|+tag> [date]` | Unmark |
| `hw list [+tag] [-tag] [--all]` | Table of habits, streaks, last 30 days |
| `hw cal [habit] [+tag] [--weeks N]` | Heatmap, 26 weeks by default |
| `hw stats [habit] [+tag]` | Streaks, rates, best weekday |
| `hw rename <habit> <new name>` | Rename |
| `hw tag <habit> +add -remove` | Change tags |
| `hw archive` / `hw unarchive <habit>` | Hide without losing history |
| `hw delete <habit> --force` | Delete, completions and all |

Global flags: `--json`, `--data <path>`, `--no-color`, `--version`, `--help`.

## Data

`$HABIT_WARRIOR_DATA`, else `$XDG_DATA_HOME/habit-warrior/habits.json`, else
`~/.local/share/habit-warrior/habits.json`.

```json
{
  "version": 1,
  "next_id": 2,
  "habits": [
    {
      "id": 1,
      "name": "workout",
      "tags": ["health", "morning"],
      "created": "2026-05-11",
      "done": ["2026-08-06", "2026-08-07"]
    }
  ]
}
```

Writes go through a temporary file and a rename, so an interrupted write cannot
truncate your history. A file that fails to parse is reported, never
overwritten. Hand-editing is fine; IDs are never reused.

Every read command takes `--json` if you would rather drive it from a script.

## Test

```
go test ./...
```
