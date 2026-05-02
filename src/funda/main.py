"""Funda Valuation TUI Tool.

Based on AKShare and Textual.
"""

from funda.app import FundaApp


def main() -> None:
    """Run the Funda TUI application."""
    app = FundaApp()
    app.run()


if __name__ == "__main__":
    main()
