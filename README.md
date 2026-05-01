# Funda

A terminal UI tool for tracking and viewing fund valuation data, built with Python, Textual, and AKShare.

## Configuration

See [funda.example.yaml](contrib/funda.example.yaml) for reference.

### Config File Search Order

1. `./funda.yaml` (current working directory)
2. `$XDG_CONFIG_HOME/funda/funda.yaml` (defaults to `~/.config/funda/funda.yaml`)

### Config Fields

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `groups` | `list[object]` | No | Fund groups shown in the group selector. Default: `[{"name": "All", "funds": []}]`. |
| `groups[].name` | `string` | Yes | Group name (for example `All`, `Teach`). |
| `groups[].funds` | `list[object]` | No | Funds under this group. Default: `[]`. |
| `groups[].funds[].code` | `string` | Yes | Fund code, for example `110003`. |
| `groups[].funds[].alias` | `string` | No | Display name override in UI. If omitted, fund code is shown. |
| `refresh_interval` | `integer` | No | Refresh interval in seconds. Default: `60`. |
| `alerts` | `object` | No | Alert settings container. |
| `alerts.highlight_threshold` | `number` | No | Reserved alert threshold in percent. Default: `2.0`. |

Notes:

- `All` is treated specially: the app auto-builds its `funds` from all non-`All` groups.
- Unknown fields are ignored.
- `alerts.highlight_threshold` is currently a reserved field and is not yet applied in UI logic.

## License

Redis Console is licensed under the [BSD-3-Clause License](https://opensource.org/licenses/BSD-3-Clause). See [LICENSE](LICENSE) for more details.
