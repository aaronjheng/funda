from dataclasses import fields
from datetime import datetime

from textual.app import ComposeResult
from textual.containers import Container, Horizontal
from textual.reactive import reactive
from textual.widgets import Label, Static

from funda.data import (
    FundData,
    get_last_trading_date,
)


class FundCard(Static):
    """Fund information card"""

    DEFAULT_CSS = """
    FundCard .fund-card {
        padding: 0 1;
        border: solid $primary-darken-2;
        height: auto;
    }

    FundCard .fund-title {
        text-style: bold;
        color: $text;
    }

    FundCard .info-row {
        height: auto;
        margin: 0;
    }

    FundCard .label {
        width: 12;
        color: $text-muted;
    }

    FundCard .value {
        color: $text;
    }

    FundCard .nav-date {
        color: $text-muted;
        text-style: dim;
        margin-left: 1;
    }

    FundCard .value.positive {
        color: #ff6b6b;
    }

    FundCard .value.negative {
        color: #51cf66;
    }

    FundCard .value.neutral {
        color: $text-muted;
    }
    """

    fund_data = reactive[FundData | None](None)

    def __init__(
        self, fund_code: str, alias: str = "", cost: float = 0.0, shares: float = 0.0
    ):
        super().__init__()
        self.fund_code = fund_code
        self.alias = alias
        self.cost = cost
        self.shares = shares
        self.fund_data = None
        self._pending_data: FundData | None = None
        self._prev_fund_data: FundData | None = None
        self._widgets: dict[str, Label] = {}

    def on_mount(self) -> None:
        if self._pending_data is not None:
            self.fund_data = self._pending_data
            self._pending_data = None

    def compose(self) -> ComposeResult:
        with Container(classes="fund-card"):
            yield Label(
                f"{self.alias or self.fund_code} ({self.fund_code})",
                classes="fund-title",
                id=f"title-{self.fund_code}",
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

    def _get_widget(self, key: str) -> Label:
        if key not in self._widgets:
            self._widgets[key] = self.query_one(f"#{key}-{self.fund_code}", Label)
        return self._widgets[key]

    def _set_change_class(self, label: Label, pct: float) -> None:
        cls = "positive" if pct > 0 else "negative" if pct < 0 else "neutral"
        if not label.has_class(cls):
            label.remove_class("positive", "negative", "neutral")
            label.add_class(cls)

    def watch_fund_data(self, data: FundData | None) -> None:
        if data is None:
            return

        old = self._prev_fund_data
        if (
            old is not None
            and data is not None
            and all(
                getattr(data, f.name) == getattr(old, f.name) for f in fields(FundData)
            )
        ):
            return
        self._prev_fund_data = data

        title_label = self._get_widget("title")
        title_label.update(
            f"{self.alias or data.name or self.fund_code} ({self.fund_code})"
        )

        last_trading_day = get_last_trading_date(datetime.now()).date()
        nav_is_current = False
        if data.nav_date:
            try:
                nav_date = datetime.strptime(data.nav_date, "%Y-%m-%d").date()
                nav_is_current = nav_date >= last_trading_day
            except ValueError:
                pass

        nav_label = self._get_widget("nav")
        nav_label.update(f"{data.nav:.4f}" if data.nav > 0 else "--")

        change_label = self._get_widget("change")
        if data.nav > 0 and data.prev_nav > 0:
            change_pct = data.day_change_percent
            change_symbol = "+" if change_pct >= 0 else ""
            change_label.update(f"{change_symbol}{change_pct:.2f}%")
            self._set_change_class(nav_label, change_pct)
            self._set_change_class(change_label, change_pct)
        else:
            change_label.update("--")
            nav_label.remove_class("positive", "negative", "neutral")
            change_label.remove_class("positive", "negative", "neutral")

        estimate_label = self._get_widget("estimate")
        if nav_is_current:
            estimate_label.update("")
        else:
            if data.estimate_nav > 0:
                est_pct = data.estimate_change_percent
                est_symbol = "+" if est_pct >= 0 else ""
                estimate_label.update(
                    f"{data.estimate_nav:.4f} ({est_symbol}{est_pct:.2f}%)"
                )
                self._set_change_class(estimate_label, est_pct)
            else:
                estimate_label.update("--")

        nav_date_label = self._get_widget("nav-date")
        nav_date_label.update(f"({data.nav_date})" if data.nav_date else "")
