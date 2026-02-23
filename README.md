# helmdex

`helmdex` is a TUI-first organizer for Helm umbrella chart instances.

## TUI

Launch the interactive dashboard:

```bash
helmdex tui
```

### Values tab

In an instance view, the **Values** tab lists the values-related files that exist in the instance directory, with a short description next to each:

- `values.default.yaml` — baseline defaults
- `values.platform.yaml` — platform overrides
- `values.set.<name>.yaml` — preset layer `<name>` (sorted)
- `values.instance.yaml` — user overrides (**editable**)
- `values.yaml` — merged output (**generated**)

Select a file to open a preview.

## YAML syntax highlighting

YAML previews are syntax-highlighted in the TUI (instance values preview, Artifact Hub “Values”, dependency detail “Default”).

## Markdown README rendering

README previews in the TUI are rendered as Markdown (to ANSI) when shown in:

- Artifact Hub detail “README”
- Dependency detail “README”

Color output is **automatically disabled** when:

- `NO_COLOR` is set (any value)
- `TERM=dumb`
