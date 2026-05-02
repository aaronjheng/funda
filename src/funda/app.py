"""Funda TUI application module."""

import asyncio
from typing import ClassVar

from textual.app import App, ComposeResult
from textual.containers import Container, Grid, Horizontal, VerticalScroll
from textual.widgets import Label, Select

from funda.config import load_config
from funda.data import (
    FundData,
    fetch_prev_nav,
    get_fund_data_full,
    load_fund_cache,
    save_fund_cache,
)
from funda.widget import FundCard


class FundaApp(App):
    """Funda Valuation TUI Application."""

    CSS = """
    Screen {
        align: center middle;
    }

    .main-container {
        width: 100%;
        height: 100%;
        padding: 1 2;
    }

    .fund-scroll {
        width: 100%;
        height: 1fr;
        scrollbar-size: 1 1;
    }

    .fund-grid {
        width: 100%;
        height: auto;
        grid-size: 2;
        grid-gutter: 0 1;
        grid-rows: auto;
    }

    .footer {
        height: 1;
        content-align: center middle;
        text-style: dim;
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

    BINDINGS: ClassVar[list[tuple[str, str, str]]] = [
        ("q", "quit", "Quit"),
        ("r", "refresh", "Refresh"),
    ]

    def __init__(self) -> None:
        """Initialize the app."""
        super().__init__()
        self.config = load_config()
        self.fund_cards: list[FundCard] = []
        self.refresh_task = None
        self.current_group = "All"

    def _get_groups(self) -> list[dict]:
        groups = self.config.get("groups", [])

        all_funds = []
        seen_codes = set()
        for group in groups:
            for fund in group.get("funds", []):
                code = fund.get("code")
                if code and code not in seen_codes:
                    all_funds.append(fund)
                    seen_codes.add(code)

        return [{"name": "All", "funds": all_funds}, *groups]

    def compose(self) -> ComposeResult:
        """Compose the UI."""
        with Container(classes="main-container"):
            groups = self._get_groups()
            group_options = [
                (f"{g['name']} ({len(g.get('funds', []))})", g["name"]) for g in groups
            ]

            with Horizontal(classes="group-selector"):
                yield Select(
                    options=group_options,
                    value="All",
                    classes="group-select",
                    id="group-select",
                    allow_blank=False,
                )

            with VerticalScroll(classes="fund-scroll", id="fund-scroll"):  # noqa: SIM117
                with Grid(classes="fund-grid", id="fund-grid"):
                    all_funds = groups[0].get("funds", [])

                    for fund_config in all_funds:
                        card = FundCard(
                            fund_code=fund_config["code"],
                            alias=fund_config.get("alias", ""),
                        )
                        self._load_card_from_cache(card)
                        self.fund_cards.append(card)
                        yield card

            yield Label("Press 'r' refresh | 'q' quit", classes="footer")

    def on_select_changed(self, event: Select.Changed) -> None:
        """Handle group selection changes."""
        if event.select.id == "group-select":
            self.current_group = str(event.value)
            self._refresh_fund_grid()

    def _refresh_fund_grid(self) -> None:
        fund_grid = self.query_one("#fund-grid", Grid)

        fund_grid.remove_children()
        self.fund_cards.clear()

        groups = self._get_groups()
        selected_funds = []
        for g in groups:
            if g["name"] == self.current_group:
                selected_funds = g.get("funds", [])
                break

        for fund_config in selected_funds:
            card = FundCard(
                fund_code=fund_config["code"],
                alias=fund_config.get("alias", ""),
            )
            self._load_card_from_cache(card)
            self.fund_cards.append(card)
            fund_grid.mount(card)

        self.refresh_task = asyncio.create_task(self.refresh_all_data())

    def _load_card_from_cache(self, card: FundCard) -> None:
        cached = load_fund_cache(card.fund_code)
        if cached:
            cached.alias = card.alias
            card.set_pending_data(cached)

    async def _fetch_card_data(
        self,
        card: FundCard,
        semaphore: asyncio.Semaphore,
    ) -> FundData | None:
        async with semaphore:
            loop = asyncio.get_running_loop()
            try:
                data = await loop.run_in_executor(
                    None,
                    get_fund_data_full,
                    card.fund_code,
                    card.alias,
                )
            except OSError, ValueError, TypeError:
                return None
        return data

    async def _fetch_prev_nav_data(
        self,
        data: FundData,
        semaphore: asyncio.Semaphore,
    ) -> FundData:
        if data.prev_nav != 0 or data.nav == 0:
            return data
        async with semaphore:
            loop = asyncio.get_running_loop()
            try:
                updated = await loop.run_in_executor(None, fetch_prev_nav, data)
            except OSError, ValueError, TypeError:
                return data
        return updated if updated and updated.prev_nav != 0 else data

    async def refresh_all_data(self) -> None:
        """Refresh data for all fund cards."""
        semaphore = asyncio.Semaphore(3)
        results = await asyncio.gather(
            *[self._fetch_card_data(card, semaphore) for card in self.fund_cards],
            return_exceptions=True,
        )
        card_data: dict[FundCard, FundData] = {
            card: result
            for card, result in zip(self.fund_cards, results, strict=True)
            if isinstance(result, FundData)
        }

        needs_prev_nav = [data for data in card_data.values() if data.prev_nav == 0]
        if needs_prev_nav:
            prev_nav_results = await asyncio.gather(
                *[
                    self._fetch_prev_nav_data(data, semaphore)
                    for data in needs_prev_nav
                ],
                return_exceptions=True,
            )
            updated_map = {}
            for orig, result in zip(needs_prev_nav, prev_nav_results, strict=True):
                if isinstance(result, FundData):
                    updated_map[id(orig)] = result
            card_data = {
                card: updated_map.get(id(data), data)
                for card, data in card_data.items()
            }

        with self.app.batch_update():
            for card, data in card_data.items():
                card.fund_data = data

        for card in self.fund_cards:
            if card.fund_data and card.fund_data.nav > 0:
                save_fund_cache(card.fund_data)

    async def action_refresh(self) -> None:
        """Handle refresh action."""
        await self.refresh_all_data()

    def on_mount(self) -> None:
        """Handle mount event."""
        self.refresh_task = asyncio.create_task(self.refresh_all_data())
