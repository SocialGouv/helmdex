# helmdex

`helmdex` is a TUI-first organizer for Helm umbrella chart instances.

## TUI

Launch the interactive dashboard:

```bash
helmdex tui
```

## YAML syntax highlighting

YAML previews are syntax-highlighted in the TUI (instance values preview, Artifact Hub “Values”, dependency detail “Default”).

## Markdown README rendering

README previews in the TUI are rendered as Markdown (to ANSI) when shown in:

- Artifact Hub detail “README”
- Dependency detail “README”

Color output is **automatically disabled** when:

- `NO_COLOR` is set (any value)
- `TERM=dumb`
