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
hw done 10 11 12             # several at once
hw done workout read mon     # several, on a given day
hw done workout --note "3 x max pullups"
hw done +morning              # everything tagged morning

hw list
hw list +health -evening
hw cal --weeks 12
hw stats workout
```

```
  50%  ██████████░░░░░░░░░░  1 of 2 today
 ID  Habit        Tags            Today  Streak      Last 30
  1  workout      health,morning  ✓          12  26/26 (100%)
  2  read a book  mind            ·           0   10/26 (38%)
```

The headline is the day so far: how many of your habits are done, as a
percentage and as raw counts. Archived habits are left out, and on the weekly
rest day it reads `Rest day` instead, since nothing is due and 0% would be a
strange way to describe a day off.

A `!` after a streak means it survives only if you do the habit today: the
count is still running from yesterday. `Last 30` counts the days you could
have done it, which is fewer than 30 once the weekly rest day is taken out.

Neither percentage rounds past the truth: a day with anything still to do
cannot show 100%, and a day with anything done cannot show 0%.

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

`done` and `undo` take several habits at once. Nothing is marked unless every
one of them names a habit, so a typo costs you the command rather than half of
it:

```
hw done 10 11 12
hw done workout read yesterday
```

A trailing date is read as a date whenever one is there and at least one habit
comes first, which leaves `hw done mon` meaning the habit called `mon` and
`hw done 3` meaning habit 3. Quote a name that contains spaces, since each
bare word is otherwise its own selector.

### Notes

`--note` attaches a note to the completions a `done` command records:

```
hw done workout --note "3 x max pullups"
hw done workout stretch yesterday --note "felt strong"
hw done +morning --note "did the whole routine"
```

The flag can go anywhere in the command, and applies to every habit that `done`
marks. Notes have no positional form: every bare argument is a habit selector,
which is what keeps a mistyped habit an error rather than a silent note.

Re-noting a day you had already marked replaces the text. `hw undo` discards
the note along with the completion it described, and refuses a note of its own.
`hw stats <habit>` shows the five most recent, and `--json` carries them all.

### Dates

`today`, `yesterday`, `tomorrow`, `3d`, `3 days ago`, `mon` (the most recent
Monday), or an ISO date like `2026-08-01`. Future days are refused.

### Sabbath

One day a week is a rest day. It is skipped, not failed: a streak runs straight
through it, and it does not count against your completion rate. Reports draw it
as `–` rather than a miss.

```
hw config                    # sabbath  sunday
hw config sabbath saturday   # rest on Saturday instead
hw config sabbath off        # no rest day; every day counts
hw config sabbath on         # back to the default, Sunday
```

Sunday is the default. Doing a habit on the rest day is never blocked and
always counts: the day joins your total and your streak, so a bonus day helps
and can never push a rate above 100%.

With Sunday resting, a habit done Friday, Saturday, Monday and Tuesday has a
streak of 4 and a rate of 100%. Turn the sabbath off and the same history is
two runs of 2 at 80%.

### Tags

`+tag` requires a tag, `-tag` excludes one, and several combine with AND.
Filters work on `list`, `cal`, `stats`, and on `done`/`undo` for marking a
whole group at once.

### Commands

| Command | What it does |
|---|---|
| `hw` | Table of habits, same as `hw list` |
| `hw help` | Print the help message |
| `hw add <name> [+tag...]` | Start tracking a habit |
| `hw done <habit...\|+tag> [date] [--note "..."]` | Mark done, one habit or several |
| `hw undo <habit...\|+tag> [date]` | Unmark |
| `hw list [+tag] [-tag] [--all]` | Daily score, then habits, streaks, last 30 days |
| `hw cal [habit] [+tag] [--weeks N]` | Heatmap, 26 weeks by default |
| `hw stats [habit] [+tag]` | Streaks, rates, best weekday |
| `hw rename <habit> <new name>` | Rename |
| `hw tag <habit> +add -remove` | Change tags |
| `hw archive` / `hw unarchive <habit>` | Hide without losing history |
| `hw delete <habit> --force` | Delete, completions and all |
| `hw config [key] [value]` | Show or change settings |

Global flags: `--json`, `--data <path>`, `--no-color`, `--version`, `--help`.

## Data

`$HABIT_WARRIOR_DATA`, else `$XDG_DATA_HOME/habit-warrior/habits.json`, else
`~/.local/share/habit-warrior/habits.json`.

```json
{
  "version": 3,
  "next_id": 2,
  "config": {
    "sabbath": "sunday"
  },
  "habits": [
    {
      "id": 1,
      "name": "workout",
      "tags": ["health", "morning"],
      "created": "2026-05-11",
      "done": ["2026-08-06", "2026-08-07"],
      "notes": {"2026-08-07": "3 x max pullups"}
    }
  ]
}
```

`config` appears once you have set something; without it the defaults apply, so
a file written by an earlier version reads correctly and rests on Sunday.
`sabbath` is a lowercase weekday name, or `"none"` to disable. `notes` appears
only on habits that have one, keyed by the day it describes.

Each schema version exists so an older `hw` refuses the file rather than
mishandling it: version 2 added `config`, which an older build would ignore
while reporting the wrong streaks, and version 3 added `notes`, which it would
drop on its next write.

Writes go through a temporary file and a rename, so an interrupted write cannot
truncate your history. A file that fails to parse is reported, never
overwritten. Hand-editing is fine; IDs are never reused.

Every read command takes `--json` if you would rather drive it from a script.

## Test

```
go test ./...
```
