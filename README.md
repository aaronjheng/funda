# Funda

A terminal UI tool for tracking and viewing fund valuation data, built with Python, Textual, and AKShare.

## Config

Config file search order:

1. `./funda.yaml` (current working directory)
2. `$XDG_CONFIG_HOME/funda/funda.yaml` (defaults to `~/.config/funda/funda.yaml`)

You can bootstrap local config from the example:

```bash
cp contrib/funda.yaml.example funda.yaml
```

## License

Redis Console is licensed under the [BSD-3-Clause License](https://opensource.org/licenses/BSD-3-Clause). See [LICENSE](LICENSE) for more details.
