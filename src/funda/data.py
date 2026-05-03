"""Funda data retrieval module."""

import copy
import json
import os
import re
import threading
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from zoneinfo import ZoneInfo

import httpx

_TZ = ZoneInfo("Asia/Shanghai")

_FETCH_ERRORS = (
    httpx.HTTPError,
    OSError,
    ValueError,
    TypeError,
    KeyError,
    IndexError,
    AttributeError,
)

# XDG cache directory
XDG_CACHE_HOME = os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache")
CACHE_DIR = Path(XDG_CACHE_HOME) / "funda"
CACHE_FILE = CACHE_DIR / "fund_data_cache.json"
# Cache for 12 hours (43200 seconds) - ensures data refreshes after market close
CACHE_DURATION = 43200

# Trading hours constants
_TRADING_START_HOUR = 9
_TRADING_END_HOUR = 15
_WEEKEND_DAYS = 5  # 0=Monday, 5=Saturday, 6=Sunday

# Data parsing constants
_MIN_NAV_COLS_FOR_PREV = 2
_DATE_PARTS_COUNT = 3

# Search results limit
_SEARCH_RESULTS_LIMIT = 10

# Memory cache for fast access
_fund_cache_state: dict[str, object] = {
    "memory": None,
    "timestamp": None,
    "dict": None,
}

# ETF data cache
_etf_cache_state: dict[str, object] = {
    "cache": None,
    "timestamp": None,
    "dict": None,
}
ETF_CACHE_DURATION = 60  # ETF real-time data caches for 60 seconds

_fund_cache_lock = threading.Lock()
_etf_cache_lock = threading.Lock()


def _get_cache_dir() -> Path:
    """Get or create cache directory following XDG spec."""
    cache_dir = Path(XDG_CACHE_HOME) / "funda"
    cache_dir.mkdir(parents=True, exist_ok=True)
    return cache_dir


def load_fund_cache(code: str) -> FundData | None:
    """Load cached FundData for a specific fund code."""
    cache_file = _get_cache_dir() / "fund_data" / f"{code}.json"
    if not cache_file.exists():
        return None
    try:
        with cache_file.open(encoding="utf-8") as f:
            data = json.load(f)
        return FundData(**data)
    except json.JSONDecodeError, TypeError, ValueError:
        return None


def save_fund_cache(data: FundData) -> None:
    """Save FundData to disk cache for a specific fund."""
    if data is None or data.nav == 0:
        return
    cache_dir = _get_cache_dir() / "fund_data"
    cache_dir.mkdir(parents=True, exist_ok=True)
    cache_file = cache_dir / f"{data.code}.json"
    try:
        with cache_file.open("w", encoding="utf-8") as f:
            json.dump(data.__dict__, f, ensure_ascii=False, indent=2)
    except OSError:
        pass


def _load_cache() -> tuple[list[dict] | None, datetime | None]:
    """Load cached fund data from disk."""
    cache_file = _get_cache_dir() / "fund_data_cache.json"

    if not cache_file.exists():
        return None, None

    try:
        with cache_file.open(encoding="utf-8") as f:
            cache_data = json.load(f)

        timestamp = datetime.fromisoformat(cache_data["timestamp"])
        if timestamp.tzinfo is None:
            timestamp = timestamp.replace(tzinfo=_TZ)

        if not _should_refresh_cache(timestamp):
            return cache_data["data"], timestamp
    except json.JSONDecodeError, KeyError, ValueError:
        pass

    return None, None


def _save_cache(data: list[dict] | None) -> None:
    """Save fund data to disk cache."""
    cache_file = _get_cache_dir() / "fund_data_cache.json"

    cache_data = {
        "timestamp": datetime.now(tz=_TZ).isoformat(),
        "data": data,
    }

    try:
        with cache_file.open("w", encoding="utf-8") as f:
            json.dump(cache_data, f, ensure_ascii=False, indent=2)
    except OSError:
        pass


def _get_cached_fund_data() -> list[dict] | None:
    """Get cached fund data or fetch new data if cache expired."""
    if (
        _fund_cache_state["memory"] is not None
        and _fund_cache_state["timestamp"] is not None
        and not _should_refresh_cache(_fund_cache_state["timestamp"])
    ):
        return _fund_cache_state["memory"]

    cached_data, timestamp = _load_cache()
    if cached_data is not None:
        _fund_cache_state["memory"] = cached_data
        _fund_cache_state["timestamp"] = timestamp
        _fund_cache_state["dict"] = None
        return cached_data

    return None


def _get_fund_data_dict(cached_data: list) -> dict:
    """Get or build the lookup dict for cached fund data."""
    if _fund_cache_state["dict"] is not None:
        return _fund_cache_state["dict"]
    _fund_cache_state["dict"] = {
        row.get("基金代码"): row for row in cached_data if row.get("基金代码")
    }
    return _fund_cache_state["dict"]


def _fetch_etf_data_sina() -> list[dict] | None:
    url = (
        "https://vip.stock.finance.sina.com.cn/quotes_service/api/jsonp.php/"
        "IO.XSRV2.CallbackList['da_yPT46_Ll7K6WD']/Market_Center.getHQNodeDataSimple"
    )
    params = {
        "page": "1",
        "num": "80",
        "sort": "changepercent",
        "asc": "0",
        "node": "etf_hq_fund",
        "_s_r_a": "init",
    }
    headers = {
        "User-Agent": _HEADERS["User-Agent"],
        "Referer": "https://vip.stock.finance.sina.com.cn/",
    }
    res = _client.get(url, params=params, headers=headers)
    match = re.search(r"\((.*)\)", res.text, re.DOTALL)
    if match:
        return json.loads(match.group(1))
    return None


def _get_etf_data() -> list[dict] | None:
    now = datetime.now(tz=_TZ)
    if (
        _etf_cache_state["cache"] is not None
        and _etf_cache_state["timestamp"] is not None
        and (now - _etf_cache_state["timestamp"]).total_seconds() < ETF_CACHE_DURATION
    ):
        return _etf_cache_state["cache"]

    with _etf_cache_lock:
        now = datetime.now(tz=_TZ)
        if (
            _etf_cache_state["cache"] is not None
            and _etf_cache_state["timestamp"] is not None
            and (now - _etf_cache_state["timestamp"]).total_seconds()
            < ETF_CACHE_DURATION
        ):
            return _etf_cache_state["cache"]

        try:
            data = _fetch_etf_data_sina()
            if data:
                _etf_cache_state["cache"] = data
                _etf_cache_state["timestamp"] = now
                _etf_cache_state["dict"] = None
                return data
        except _FETCH_ERRORS:
            pass

        return None


def _get_etf_dict() -> dict:
    if _etf_cache_state["dict"] is not None:
        return _etf_cache_state["dict"]
    data = _get_etf_data()
    if not data:
        return {}
    _etf_cache_state["dict"] = {
        row.get("symbol", ""): row for row in data if row.get("symbol")
    }
    return _etf_cache_state["dict"]


_HEADERS = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "Referer": "https://fund.eastmoney.com/",
}

_client = httpx.Client(headers=_HEADERS, timeout=30)


def _fetch_fund_daily_em() -> list[dict] | None:
    url = "https://fund.eastmoney.com/Data/Fund_JJJZ_Data.aspx"
    params = {
        "t": "1",
        "lx": "1",
        "letter": "",
        "gsid": "",
        "text": "",
        "sort": "zdf,desc",
        "page": "1,50000",
        "dt": "1580914040623",
        "atfc": "",
        "onlySale": "0",
    }
    res = _client.get(url, params=params)
    text = res.text.strip().removeprefix("var db=")
    json_str = re.sub(r"([a-zA-Z_]\w*)\s*:", r'"\1":', text)
    json_str = re.sub(r",\s*([}\]])", r"\1", json_str)
    data = json.loads(json_str)
    show_day = data["showday"]
    return [
        {
            "基金代码": item[0],
            "基金简称": item[1],
            f"{show_day[0]}-单位净值": item[3],
            f"{show_day[0]}-累计净值": item[4],
            f"{show_day[1]}-单位净值": item[5],
            f"{show_day[1]}-累计净值": item[6],
            "日增长值": item[7],
            "日增长率": item[8],
        }
        for item in data["datas"]
    ]


def _fetch_and_cache_fund_data() -> list[dict] | None:
    with _fund_cache_lock:
        if (
            _fund_cache_state["memory"] is not None
            and _fund_cache_state["timestamp"] is not None
            and not _should_refresh_cache(_fund_cache_state["timestamp"])
        ):
            return _fund_cache_state["memory"]

        try:
            data_dict = _fetch_fund_daily_em()
            if data_dict:
                _save_cache(data_dict)
                _fund_cache_state["memory"] = data_dict
                _fund_cache_state["timestamp"] = datetime.now(tz=_TZ)
                _fund_cache_state["dict"] = None
                return data_dict
        except _FETCH_ERRORS:
            pass

        return None


@dataclass
class FundData:
    """Fund data class."""

    code: str
    alias: str
    name: str = ""  # Net Asset Value (latest trading day)
    nav: float = 0.0  # Net Asset Value (latest trading day)
    acc_nav: float = 0.0  # Accumulated NAV
    nav_date: str = ""  # Date of the NAV (latest trading day)
    day_change: float = 0.0  # Daily change
    estimate_nav: float = 0.0  # Estimated NAV (real-time)
    estimate_time: str = ""  # Estimate time
    prev_nav: float = 0.0  # Previous trading day's NAV

    @property
    def day_change_percent(self) -> float:
        """Calculate daily change percentage."""
        if self.prev_nav == 0:
            return 0.0
        return ((self.nav - self.prev_nav) / self.prev_nav) * 100

    @property
    def estimate_change_percent(self) -> float:
        """Calculate estimated change percentage."""
        if self.nav == 0:
            return 0.0
        return ((self.estimate_nav - self.nav) / self.nav) * 100


def _should_refresh_cache(timestamp: datetime) -> bool:
    """Check if cache should be refreshed based on trading hours."""
    age_seconds = (datetime.now(tz=_TZ) - timestamp).total_seconds()
    now = datetime.now(tz=_TZ)

    # During trading hours (9:30-15:00 on weekdays), use shorter cache
    if is_trading_day(now) and _TRADING_START_HOUR <= now.hour < _TRADING_END_HOUR:
        # Use 5-minute cache during trading hours
        trading_cache_duration = 300
        if age_seconds > trading_cache_duration:
            return True
    elif age_seconds >= CACHE_DURATION:
        return True

    # After market close (15:00+), check if cache is from earlier date
    if is_trading_day(now) and now.hour >= _TRADING_END_HOUR:
        cache_date = timestamp.date()
        today_date = now.date()

        if cache_date < today_date:
            last_trading_day = get_last_trading_date(today_date)
            if cache_date < last_trading_day.date():
                return True

    return False


def is_trading_day(date: datetime) -> bool:
    """Check if given date is a trading day (not weekend).

    Args:
        date: Date to check

    Returns:
        True if trading day

    """
    # 0=Monday, 5=Saturday, 6=Sunday
    return date.weekday() < _WEEKEND_DAYS


def is_trading_hours(now: datetime | None = None) -> bool:
    """Check if current time is within A-share trading hours on a trading day.

    Args:
        now: Current time (defaults to now in Asia/Shanghai)

    Returns:
        True if within trading hours on a trading day

    """
    if now is None:
        now = datetime.now(tz=_TZ)
    if not is_trading_day(now):
        return False
    return _TRADING_START_HOUR <= now.hour < _TRADING_END_HOUR


def get_last_trading_date(date: datetime) -> datetime:
    """Get the last trading date (handles weekends).

    Args:
        date: Current date

    Returns:
        Last trading date

    """
    # Go back until we find a trading day
    last_date = date
    while not is_trading_day(last_date):
        last_date -= timedelta(days=1)
    return last_date


def _parse_open_fund_row(
    row: dict,
    code: str,
    alias: str,
    today: datetime,
) -> FundData:
    """Parse open fund row into FundData."""
    fund = FundData(code=code, alias=alias)
    fund.name = str(row.get("基金简称", alias or code))
    nav_cols = sorted(key for key in row if "单位净值" in key and row.get(key))
    if nav_cols:
        nav_col = nav_cols[-1]
        fund.nav = float(row.get(nav_col, 0) or 0)
        if len(nav_cols) >= _MIN_NAV_COLS_FOR_PREV:
            prev_nav_col = nav_cols[-_MIN_NAV_COLS_FOR_PREV]
            fund.prev_nav = float(row.get(prev_nav_col, 0) or 0)

        fund.day_change = fund.nav - fund.prev_nav if fund.prev_nav else 0.0

        acc_nav_col = nav_col.replace("单位", "累计")
        fund.acc_nav = float(row.get(acc_nav_col, 0) or 0)
        if "-" in nav_col:
            parts = nav_col.split("-")
            fund.nav_date = (
                "-".join(parts[:_DATE_PARTS_COUNT])
                if len(parts) >= _DATE_PARTS_COUNT
                else parts[0]
            )
        else:
            fund.nav_date = today.strftime("%Y-%m-%d")

    if fund.prev_nav == 0 and fund.nav > 0:
        try:
            data = _fetch_fund_info_em(code)
            if data and len(data) >= _MIN_NAV_COLS_FOR_PREV:
                fund.prev_nav = float(data[-_MIN_NAV_COLS_FOR_PREV]["y"])
                fund.day_change = fund.nav - fund.prev_nav
        except _FETCH_ERRORS:
            pass

    return fund


def _parse_etf_row(
    row: dict,
    code: str,
    alias: str,
    today: datetime,
) -> FundData:
    """Parse ETF row into FundData."""
    fund = FundData(code=code, alias=alias)
    fund.name = str(row.get("name", alias or code))
    fund.estimate_nav = float(row.get("trade", 0) or 0)
    fund.nav = fund.estimate_nav
    prev_close = float(row.get("settlement", 0) or 0)
    if prev_close > 0 and fund.estimate_nav > 0:
        fund.day_change = fund.estimate_nav - prev_close
    fund.nav_date = today.strftime("%Y-%m-%d")
    return fund


def get_fund_data(code: str, alias: str = "") -> FundData:
    """Get fund data.

    This function handles both trading and non-trading days:
    - On trading days with active market: returns real-time data
    - On non-trading days: returns latest historical data with correct date

    Args:
        code: Fund code
        alias: Fund alias

    Returns:
        FundData: Fund data

    """
    today = datetime.now(tz=_TZ)

    try:
        cached_data = _get_cached_fund_data()
        if cached_data is None:
            cached_data = _fetch_and_cache_fund_data()

        if cached_data is not None:
            cached_data_dict = _get_fund_data_dict(cached_data)
            row = cached_data_dict.get(code)
            if row:
                return _parse_open_fund_row(row, code, alias, today)
    except _FETCH_ERRORS:
        pass

    try:
        etf_dict = _get_etf_dict()
        for prefix in ["sh", "sz"]:
            row = etf_dict.get(f"{prefix}{code}")
            if row is not None:
                return _parse_etf_row(row, code, alias, today)
    except _FETCH_ERRORS:
        pass

    return FundData(code=code, alias=alias)


def _lookup_etf_estimate(code: str) -> tuple[float, str]:
    etf_dict = _get_etf_dict()
    for prefix in ["sh", "sz"]:
        row = etf_dict.get(f"{prefix}{code}")
        if row is not None:
            latest_price = float(row.get("trade", 0) or 0)
            update_time = datetime.now(tz=_TZ).strftime("%H:%M:%S")
            return latest_price, update_time
    return 0.0, ""


def _lookup_cached_estimate(code: str) -> tuple[float, str]:
    cached_data = _get_cached_fund_data()
    if cached_data is None:
        cached_data = _fetch_and_cache_fund_data()
    if cached_data is None:
        return 0.0, ""
    cached_data_dict = _get_fund_data_dict(cached_data)
    row = cached_data_dict.get(code)
    if not row:
        return 0.0, ""
    nav_cols = sorted([key for key in row if "单位净值" in key])
    if not nav_cols:
        return 0.0, ""
    nav_col = nav_cols[-1]
    latest_price = float(row.get(nav_col, 0) or 0)
    daily_growth_rate = row.get("日增长率", 0)
    if daily_growth_rate:
        try:
            growth_pct = float(daily_growth_rate.strip("%"))
            estimate = latest_price * (1 + growth_pct / 100)
            update_time = datetime.now(tz=_TZ).strftime("%H:%M:%S")
        except ValueError, AttributeError:
            pass
        else:
            return estimate, update_time
    update_time = datetime.now(tz=_TZ).strftime("%H:%M:%S")
    return latest_price, update_time


def _add_estimate(fund: FundData, code: str) -> None:
    """Add real-time estimate to FundData if NAV is available and market is open."""
    if fund.nav <= 0:
        return
    if not is_trading_hours():
        return
    estimate, update_time = _lookup_etf_estimate(code)
    if estimate > 0:
        fund.estimate_nav = estimate
        fund.estimate_time = update_time
    else:
        estimate, update_time = _lookup_cached_estimate(code)
        if estimate > 0:
            fund.estimate_nav = estimate
            fund.estimate_time = update_time


def get_fund_data_full(code: str, alias: str = "") -> FundData:
    """Get full fund data including real-time estimates."""
    today = datetime.now(tz=_TZ)

    try:
        cached_data = _get_cached_fund_data()
        if cached_data is None:
            cached_data = _fetch_and_cache_fund_data()

        if cached_data is not None:
            cached_data_dict = _get_fund_data_dict(cached_data)
            row = cached_data_dict.get(code)
            if row:
                fund = _parse_open_fund_row(row, code, alias, today)
                _add_estimate(fund, code)
                return fund
    except _FETCH_ERRORS:
        pass

    try:
        etf_dict = _get_etf_dict()
        for prefix in ["sh", "sz"]:
            row = etf_dict.get(f"{prefix}{code}")
            if row is not None:
                fund = _parse_etf_row(row, code, alias, today)
                if is_trading_hours(today):
                    fund.estimate_time = datetime.now(
                        tz=_TZ,
                    ).strftime("%H:%M:%S")
                else:
                    fund.estimate_nav = 0.0
                    fund.estimate_time = ""
                return fund
    except _FETCH_ERRORS:
        pass

    return FundData(code=code, alias=alias)


def get_realtime_estimate(code: str) -> tuple[float, str]:
    """Get real-time estimate for a fund."""
    estimate, update_time = _lookup_etf_estimate(code)
    if estimate > 0:
        return estimate, update_time
    return _lookup_cached_estimate(code)


def _fetch_fund_info_em(code: str) -> list[dict] | None:
    url = f"https://fund.eastmoney.com/pingzhongdata/{code}.js"
    res = _client.get(url)
    match = re.search(r"var Data_netWorthTrend\s*=\s*(\[.*?\]);", res.text, re.DOTALL)
    if match:
        return json.loads(match.group(1))
    return None


def fetch_prev_nav(fund: FundData) -> FundData:
    """Fetch previous NAV for a fund if missing."""
    if fund.prev_nav != 0 or fund.nav == 0:
        return fund
    try:
        data = _fetch_fund_info_em(fund.code)
        if data and len(data) >= _MIN_NAV_COLS_FOR_PREV:
            updated = copy.copy(fund)
            updated.prev_nav = float(data[-_MIN_NAV_COLS_FOR_PREV]["y"])
            updated.day_change = updated.nav - updated.prev_nav
            return updated
    except _FETCH_ERRORS:
        pass
    return fund


def search_fund(keyword: str) -> list[dict]:
    """Search fund.

    Args:
        keyword: Search keyword

    Returns:
        List of funds

    """
    results = []

    # Try open fund data first (for non-ETF funds) using cache
    try:
        # Try to get cached data first
        cached_data = _get_cached_fund_data()

        # If no cache exists, fetch and cache new data
        if cached_data is None:
            cached_data = _fetch_and_cache_fund_data()

        if cached_data is not None:
            # Search in cached data
            matched = [
                row
                for row in cached_data
                if keyword in str(row.get("基金代码", ""))
                or keyword in str(row.get("基金简称", ""))
            ][:_SEARCH_RESULTS_LIMIT]  # Limit to 10 results

            for row in matched:
                nav_col = next((key for key in row if "单位净值" in key), None)
                results.append(
                    {
                        "基金代码": row.get("基金代码"),
                        "基金名称": row.get("基金简称"),
                        "最新价": row.get(nav_col, "") if nav_col else "",
                        "涨跌幅": row.get("日增长率", ""),
                    },
                )
            if results:
                return results
    except _FETCH_ERRORS:
        pass

    # Fallback to ETF data source
    try:
        etf_dict = _get_etf_dict()
        for row in etf_dict.values():
            code_val = row.get("code", "")
            name_val = row.get("name", "")
            if keyword in code_val or keyword in name_val:
                results.append(
                    {
                        "基金代码": code_val,
                        "基金名称": name_val,
                        "最新价": row.get("trade", ""),
                        "涨跌幅": row.get("changepercent", ""),
                    },
                )
                if len(results) >= _SEARCH_RESULTS_LIMIT:
                    break
    except _FETCH_ERRORS:
        pass

    return results
