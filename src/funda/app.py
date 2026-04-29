import asyncio
from datetime import datetime

from textual.app import App, ComposeResult
from textual.containers import Container, Grid, Horizontal, VerticalScroll
from textual.widgets import Label, Select

from funda.config import load_config
from funda.data import get_fund_data, get_realtime_estimate
from funda.widget import FundCard


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

        return [{"name": "全部", "funds": all_funds}] + groups

    def compose(self) -> ComposeResult:
        with Container(classes="main-container"):
            current_date = datetime.now().strftime("%Y-%m-%d %A")
            yield Label(f"📅 {current_date}", classes="date-display")

            groups = self._get_groups()
            group_options = [
                (f"{g['name']} ({len(g.get('funds', []))})", g["name"]) for g in groups
            ]

            with Horizontal(classes="group-selector"):
                yield Select(
                    options=group_options,
                    value="全部",
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
                        self.fund_cards.append(card)
                        yield card

            yield Label("Press 'r' refresh | 'q' quit", classes="footer")

    def on_select_changed(self, event: Select.Changed) -> None:
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
            self.fund_cards.append(card)
            fund_grid.mount(card)

        asyncio.create_task(self.refresh_all_data())

    async def refresh_all_data(self) -> None:
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
        await self.refresh_all_data()
