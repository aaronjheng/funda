"""Funda Valuation TUI Tool
Based on AKShare and Textual
"""

import asyncio
import os
from copy import deepcopy
from datetime import datetime
from pathlib import Path

import yaml
from textual.app import App, ComposeResult
from textual.containers import Container, Grid, Horizontal
from textual.reactive import reactive
from textual.widgets import Label, Select, Static

from funda.data import (
    FundData,
    get_fund_data,
    get_last_trading_date,
    get_realtime_estimate,
)

DEFAULT_CONFIG = {
    "groups": [{"name": "全部", "funds": []}],
    "refresh_interval": 60,
    "alerts": {"highlight_threshold": 2.0},
}


def _resolve_config_path() -> Path | None:
    """Resolve config path with priority: CWD then XDG config directory."""
    cwd_config = Path.cwd() / "funda.yaml"
    if cwd_config.is_file():
        return cwd_config

    xdg_config_home = Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config"))
    xdg_config = xdg_config_home / "funda" / "funda.yaml"
    if xdg_config.is_file():
        return xdg_config

    return None


def load_config() -> dict:
    """Load configuration file"""
    config_path = _resolve_config_path()
    if config_path is None:
        return deepcopy(DEFAULT_CONFIG)

    try:
        with config_path.open(encoding="utf-8") as f:
            data = yaml.safe_load(f) or {}
            if isinstance(data, dict):
                return data
            raise ValueError("Config file content must be a mapping")
    except Exception as e:
        print(f"Failed to load config: {e}")
        return deepcopy(DEFAULT_CONFIG)


class FundCard(Static):
    """Fund information card"""

    fund_data = reactive[FundData | None](None)

    def __init__(
        self, fund_code: str, alias: str = "", cost: float = 0.0, shares: float = 0.0
    ):
        super().__init__()
        self.fund_code = fund_code
        self.alias = alias
        self.cost = cost  # Cost price
        self.shares = shares  # Shares
        self.fund_data = None

    def compose(self) -> ComposeResult:
        with Container(classes="fund-card"):
            yield Label(
                f"{self.alias or self.fund_code} ({self.fund_code})",
                classes="fund-title",
            )

            with Horizontal(classes="info-row"):
                yield Label("最新净值:", classes="label")
                yield Label("加载中...", id=f"nav-{self.fund_code}", classes="value")
                yield Label("", id=f"nav-date-{self.fund_code}", classes="nav-date")

            with Horizontal(classes="info-row"):
                yield Label("日涨跌:", classes="label")
                yield Label("--", id=f"change-{self.fund_code}", classes="value")

            with Horizontal(classes="info-row"):
                yield Label("实时估值:", classes="label")
                yield Label("--", id=f"estimate-{self.fund_code}", classes="value")

    def watch_fund_data(self, data: FundData | None) -> None:
        """Update UI when fund data changes"""
        if data is None:
            return

        # Determine if NAV is up-to-date (matches latest trading day)
        last_trading_day = get_last_trading_date(datetime.now()).date()
        nav_is_current = False
        if data.nav_date:
            try:
                nav_date = datetime.strptime(data.nav_date, "%Y-%m-%d").date()
                nav_is_current = nav_date >= last_trading_day
            except ValueError:
                pass

        # Update NAV (confirmed NAV)
        nav_label = self.query_one(f"#nav-{self.fund_code}", Label)
        if data.nav > 0:
            nav_label.update(f"{data.nav:.4f}")
        else:
            nav_label.update("--")

        # Update daily change (based on confirmed NAV)
        change_label = self.query_one(f"#change-{self.fund_code}", Label)
        if data.nav > 0:
            change_pct = data.day_change_percent
            change_symbol = "+" if change_pct >= 0 else ""
            change_label.update(f"{change_symbol}{change_pct:.2f}%")
            nav_label.remove_class("positive", "negative", "neutral")
            change_label.remove_class("positive", "negative", "neutral")
            if change_pct > 0:
                nav_label.add_class("positive")
                change_label.add_class("positive")
            elif change_pct < 0:
                nav_label.add_class("negative")
                change_label.add_class("negative")
            else:
                nav_label.add_class("neutral")
                change_label.add_class("neutral")
        else:
            change_label.update("--")

        # Update real-time estimate
        estimate_label = self.query_one(f"#estimate-{self.fund_code}", Label)
        if nav_is_current:
            # NAV is up-to-date: hide estimate field
            estimate_label.update("")
        else:
            # NAV is outdated: show real-time estimate with change
            if data.estimate_nav > 0:
                est_pct = data.estimate_change_percent
                est_symbol = "+" if est_pct >= 0 else ""
                estimate_label.update(
                    f"{data.estimate_nav:.4f} ({est_symbol}{est_pct:.2f}%)"
                )
                estimate_label.remove_class("positive", "negative", "neutral")
                if est_pct > 0:
                    estimate_label.add_class("positive")
                elif est_pct < 0:
                    estimate_label.add_class("negative")
                else:
                    estimate_label.add_class("neutral")
            else:
                estimate_label.update("--")

        # Update NAV date
        nav_date_label = self.query_one(f"#nav-date-{self.fund_code}", Label)
        if data.nav_date:
            nav_date_label.update(f"({data.nav_date})")
        else:
            nav_date_label.update("")


class FundaApp(App):
    """Funda Valuation TUI Application"""

    CSS = """
    Screen {
        align: center middle;
    }

    .main-container {
        width: 100%;
        height: 100%;
        padding: 1 2;
    }

    .header {
        height: 3;
        content-align: center middle;
        text-style: bold;
        background: $surface;
    }

    .fund-grid {
        width: 100%;
        height: 1fr;
        grid-size: 2;
        grid-gutter: 1;
    }

    .fund-card {
        padding: 0 1;
        border: solid $primary;
        height: auto;
    }

    .fund-title {
        text-style: bold;
        color: $text;
    }

    .fund-code {
        color: $text-muted;
        text-style: dim;
    }

    .info-row {
        height: auto;
        margin: 0;
    }

    .label {
        width: 12;
        color: $text-muted;
    }

    .value {
        color: $text;
    }

    .nav-date {
        color: $text-muted;
        text-style: dim;
        margin-left: 1;
    }

    .positive {
        color: #ff6b6b;
    }

    .negative {
        color: #51cf66;
    }

    .neutral {
        color: $text-muted;
    }

    .update-time {
        text-style: dim;
        color: $text-muted;
    }

    .footer {
        height: 1;
        content-align: center middle;
        text-style: dim;
    }

    .date-display {
        height: 1;
        content-align: center middle;
        text-style: bold;
        color: $text;
        margin: 1 0;
    }

    .group-selector {
        height: auto;
        margin: 1 0;
        align: center middle;
    }

    .group-select {
        width: 30;
    }
    """

    BINDINGS = [
        ("q", "quit", "Quit"),
        ("r", "refresh", "Refresh"),
    ]

    def __init__(self):
        super().__init__()
        self.config = load_config()
        self.fund_cards: list[FundCard] = []
        self.refresh_task = None
        self.current_group = "全部"

    def _get_all_groups_with_all(self) -> list[dict]:
        """Get all groups with '全部' as first item"""
        groups = self.config.get("groups", [])

        # Collect all unique funds from all groups (skip "All" group itself)
        all_funds = []
        seen_codes = set()
        for group in groups:
            if group["name"] == "全部":
                continue
            for fund in group.get("funds", []):
                code = fund.get("code")
                if code and code not in seen_codes:
                    all_funds.append(fund)
                    seen_codes.add(code)

        # Update or create 'All' group with all funds
        result = []
        for group in groups:
            if group["name"] == "全部":
                result.append({"name": "全部", "funds": all_funds})
            else:
                result.append(group)

        return result

    def compose(self) -> ComposeResult:
        with Container(classes="main-container"):
            # Display current date
            current_date = datetime.now().strftime("%Y-%m-%d %A")
            yield Label(f"📅 {current_date}", classes="date-display")

            # Group selector
            groups_with_all = self._get_all_groups_with_all()
            group_options = [(g["name"], g["name"]) for g in groups_with_all]

            with Horizontal(classes="group-selector"):
                yield Select(
                    options=group_options,
                    value="全部",
                    classes="group-select",
                    id="group-select",
                    allow_blank=False,
                )

            with Grid(classes="fund-grid", id="fund-grid"):
                # Initial load with "All" group
                all_funds = groups_with_all[0].get("funds", [])

                for fund_config in all_funds:
                    card = FundCard(
                        fund_code=fund_config["code"],
                        alias=fund_config.get("alias", ""),
                    )
                    self.fund_cards.append(card)
                    yield card

            yield Label("Press 'r' refresh | 'q' quit", classes="footer")

    def on_select_changed(self, event: Select.Changed) -> None:
        """Handle group selection change"""
        if event.select.id == "group-select":
            self.current_group = str(event.value)
            self._refresh_fund_grid()

    def _refresh_fund_grid(self) -> None:
        """Refresh the fund grid based on selected group"""
        # Find the fund grid
        fund_grid = self.query_one("#fund-grid", Grid)

        # Clear existing cards
        fund_grid.remove_children()
        self.fund_cards.clear()

        # Get funds for selected group (including auto-generated "All")
        groups_with_all = self._get_all_groups_with_all()
        selected_funds = []
        for g in groups_with_all:
            if g["name"] == self.current_group:
                selected_funds = g.get("funds", [])
                break

        # Add new cards
        for fund_config in selected_funds:
            card = FundCard(
                fund_code=fund_config["code"],
                alias=fund_config.get("alias", ""),
            )
            self.fund_cards.append(card)
            fund_grid.mount(card)

        # Refresh data
        asyncio.create_task(self.refresh_all_data())

    async def refresh_all_data(self) -> None:
        """Refresh all fund data"""
        for card in self.fund_cards:
            try:
                loop = asyncio.get_running_loop()
                data = await loop.run_in_executor(
                    None, get_fund_data, card.fund_code, card.alias
                )

                if data:
                    estimate, update_time = await loop.run_in_executor(
                        None, get_realtime_estimate, card.fund_code
                    )
                    if estimate > 0:
                        data.estimate_nav = estimate
                        data.estimate_time = update_time

                    card.fund_data = data
            except Exception as e:
                print(f"Failed to refresh fund {card.fund_code}: {e}")

    async def action_refresh(self) -> None:
        """Manual refresh"""
        await self.refresh_all_data()


def main():
    """Main function"""
    app = FundaApp()
    app.run()


if __name__ == "__main__":
    main()
