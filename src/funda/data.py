"""Funda data retrieval module"""

import json
import os
import threading
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path

# XDG cache directory
XDG_CACHE_HOME = os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache")
CACHE_DIR = Path(XDG_CACHE_HOME) / "funda"
CACHE_FILE = CACHE_DIR / "fund_data_cache.json"
CACHE_DURATION = 43200  # Cache for 12 hours (43200 seconds) - ensures data refreshes after market close

# Memory cache for fast access
_memory_cache = None
_memory_cache_timestamp = None
_memory_cache_dict = None

# ETF data cache
_etf_cache = None
_etf_cache_timestamp = None
ETF_CACHE_DURATION = 60  # ETF real-time data caches for 60 seconds

_fund_cache_lock = threading.Lock()
_etf_cache_lock = threading.Lock()

_etf_dict = None


def _get_cache_dir() -> Path:
    """Get or create cache directory following XDG spec"""
    cache_dir = Path(XDG_CACHE_HOME) / "funda"
    cache_dir.mkdir(parents=True, exist_ok=True)
    return cache_dir


def _load_cache():
    """Load cached fund data from disk"""
    cache_file = _get_cache_dir() / "fund_data_cache.json"

    if not cache_file.exists():
        return None, None

    try:
        with open(cache_file, encoding="utf-8") as f:
            cache_data = json.load(f)

        timestamp = datetime.fromisoformat(cache_data["timestamp"])

        if not _should_refresh_cache(timestamp):
            return cache_data["data"], timestamp
    except json.JSONDecodeError, KeyError, ValueError:
        pass

    return None, None


def _save_cache(data):
    """Save fund data to disk cache"""
    cache_file = _get_cache_dir() / "fund_data_cache.json"

    cache_data = {
        "timestamp": datetime.now().isoformat(),
        "data": data,
    }

    try:
        with open(cache_file, "w", encoding="utf-8") as f:
            json.dump(cache_data, f, ensure_ascii=False, indent=2)
    except OSError:
        pass


def _get_cached_fund_data():
    """Get cached fund data or fetch new data if cache expired"""
    global _memory_cache, _memory_cache_timestamp, _memory_cache_dict

    if (
        _memory_cache is not None
        and _memory_cache_timestamp is not None
        and not _should_refresh_cache(_memory_cache_timestamp)
    ):
        return _memory_cache

    cached_data, timestamp = _load_cache()
    if cached_data is not None:
        print("Loaded data from disk cache")
        _memory_cache = cached_data
        _memory_cache_timestamp = timestamp
        _memory_cache_dict = None
        return cached_data

    return None


def _get_fund_data_dict(cached_data: list) -> dict:
    """Get or build the lookup dict for cached fund data"""
    global _memory_cache_dict
    if _memory_cache_dict is not None:
        return _memory_cache_dict
    _memory_cache_dict = {
        row.get("基金代码"): row for row in cached_data if row.get("基金代码")
    }
    return _memory_cache_dict


def _get_etf_data():
    """Get ETF data with short-term memory cache"""
    global _etf_cache, _etf_cache_timestamp, _etf_dict

    now = datetime.now()
    if (
        _etf_cache is not None
        and _etf_cache_timestamp is not None
        and (now - _etf_cache_timestamp).total_seconds() < ETF_CACHE_DURATION
    ):
        return _etf_cache

    with _etf_cache_lock:
        now = datetime.now()
        if (
            _etf_cache is not None
            and _etf_cache_timestamp is not None
            and (now - _etf_cache_timestamp).total_seconds() < ETF_CACHE_DURATION
        ):
            return _etf_cache

        try:
            import akshare as ak

            df = ak.fund_etf_category_sina(symbol="ETF基金")
            if df is not None and not df.empty:
                _etf_cache = df
                _etf_cache_timestamp = now
                _etf_dict = None
                return df
        except Exception:
            pass

        return None


def _get_etf_dict() -> dict:
    """Get or build the lookup dict for ETF data"""
    global _etf_dict
    if _etf_dict is not None:
        return _etf_dict
    df = _get_etf_data()
    if df is None or df.empty:
        return {}
    _etf_dict = {row["代码"]: row for _, row in df.iterrows() if row.get("代码")}
    return _etf_dict


def _fetch_and_cache_fund_data():
    """Fetch new fund data from API and cache it"""
    global _memory_cache, _memory_cache_timestamp, _memory_cache_dict

    with _fund_cache_lock:
        if (
            _memory_cache is not None
            and _memory_cache_timestamp is not None
            and not _should_refresh_cache(_memory_cache_timestamp)
        ):
            return _memory_cache

        try:
            import akshare as ak

            print("Fetching data from AKShare API...")
            df = ak.fund_open_fund_daily_em()
            if df is not None and not df.empty:
                data_dict = df.to_dict(orient="records")
                _save_cache(data_dict)
                _memory_cache = data_dict
                _memory_cache_timestamp = datetime.now()
                _memory_cache_dict = None
                print(f"Fetched and cached {len(data_dict)} funds")
                return data_dict
        except Exception as e:
            print(f"Failed to fetch data: {e}")

        return None


@dataclass
class FundData:
    """Fund data class"""

    code: str
    alias: str
    name: str = ""
    nav: float = 0.0  # Net Asset Value (latest trading day)
    acc_nav: float = 0.0  # Accumulated NAV
    nav_date: str = ""  # Date of the NAV (latest trading day)
    day_change: float = 0.0  # Daily change
    estimate_nav: float = 0.0  # Estimated NAV (real-time)
    estimate_time: str = ""  # Estimate time
    prev_nav: float = 0.0  # Previous trading day's NAV

    @property
    def day_change_percent(self) -> float:
        """Calculate daily change percentage"""
        if self.prev_nav == 0:
            return 0.0
        return ((self.nav - self.prev_nav) / self.prev_nav) * 100

    @property
    def estimate_change_percent(self) -> float:
        """Calculate estimated change percentage"""
        if self.nav == 0:
            return 0.0
        return ((self.estimate_nav - self.nav) / self.nav) * 100


def _should_refresh_cache(timestamp: datetime) -> bool:
    """Check if cache should be refreshed based on trading hours"""
    age_seconds = (datetime.now() - timestamp).total_seconds()
    now = datetime.now()

    # During trading hours (9:30-15:00 on weekdays), use shorter cache
    if is_trading_day(now) and 9 <= now.hour < 15:
        # Use 5-minute cache during trading hours
        trading_cache_duration = 300
        if age_seconds > trading_cache_duration:
            return True
    elif age_seconds >= CACHE_DURATION:
        return True

    # After market close (15:00+), check if cache is from earlier date
    if is_trading_day(now) and now.hour >= 15:
        cache_date = timestamp.date()
        today_date = now.date()

        if cache_date < today_date:
            last_trading_day = get_last_trading_date(today_date)
            if cache_date < last_trading_day.date():
                return True

    return False


def is_trading_day(date: datetime) -> bool:
    """Check if given date is a trading day (not weekend)

    Args:
        date: Date to check

    Returns:
        True if trading day
    """
    # 0=Monday, 5=Saturday, 6=Sunday
    return date.weekday() < 5


def get_last_trading_date(date: datetime) -> datetime:
    """Get the last trading date (handles weekends)

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


def get_fund_data(code: str, alias: str = "") -> FundData:
    """Get fund data

    This function handles both trading and non-trading days:
    - On trading days with active market: returns real-time data
    - On non-trading days: returns latest historical data with correct date

    Args:
        code: Fund code
        alias: Fund alias

    Returns:
        FundData: Fund data
    """
    fund = FundData(code=code, alias=alias)
    today = datetime.now()

    # Try to fetch data from cached open fund daily data (for non-ETF funds)
    try:
        # First try to get cached data
        cached_data = _get_cached_fund_data()

        # If no cache exists, fetch and cache new data
        if cached_data is None:
            cached_data = _fetch_and_cache_fund_data()

        if cached_data is not None:
            cached_data_dict = _get_fund_data_dict(cached_data)
            row = cached_data_dict.get(code)
            if row:
                fund.name = str(row.get("基金简称", alias or code))
                nav_cols = sorted(
                    key for key in row if "单位净值" in key and row.get(key)
                )
                if nav_cols:
                    nav_col = nav_cols[-1]
                    fund.nav = float(row.get(nav_col, 0) or 0)
                    if len(nav_cols) >= 2:
                        prev_nav_col = nav_cols[-2]
                        fund.prev_nav = float(row.get(prev_nav_col, 0) or 0)

                    fund.day_change = fund.nav - fund.prev_nav if fund.prev_nav else 0.0

                    acc_nav_col = nav_col.replace("单位", "累计")
                    fund.acc_nav = float(row.get(acc_nav_col, 0) or 0)
                    if "-" in nav_col:
                        parts = nav_col.split("-")
                        fund.nav_date = (
                            "-".join(parts[:3]) if len(parts) >= 3 else parts[0]
                        )
                    else:
                        fund.nav_date = today.strftime("%Y-%m-%d")

                if fund.prev_nav == 0 and fund.nav > 0:
                    try:
                        import akshare as ak

                        hist_df = ak.fund_open_fund_info_em(
                            symbol=code, indicator="单位净值走势"
                        )
                        if hist_df is not None and len(hist_df) >= 2:
                            fund.prev_nav = float(hist_df.iloc[-2]["单位净值"])
                            fund.day_change = fund.nav - fund.prev_nav
                    except Exception:
                        pass

                return fund
    except Exception:
        pass

    # Fallback to ETF data source for ETF funds
    try:
        df = _get_etf_data()
        if df is not None and not df.empty:
            for prefix in ["sz", "sh"]:
                sina_code = f"{prefix}{code}"
                row = df[df["代码"] == sina_code]
                if not row.empty:
                    row_data = row.iloc[0]
                    fund.name = str(row_data.get("名称", alias or code))
                    fund.estimate_nav = float(row_data.get("最新价", 0) or 0)
                    fund.nav = fund.estimate_nav
                    prev_close = float(row_data.get("昨收", 0) or 0)
                    if prev_close > 0 and fund.estimate_nav > 0:
                        fund.day_change = fund.estimate_nav - prev_close
                    fund.nav_date = today.strftime("%Y-%m-%d")
                    return fund
    except Exception:
        pass

    return fund


def _lookup_etf_estimate(code: str) -> tuple[float, str]:
    etf_dict = _get_etf_dict()
    for prefix in ["sz", "sh"]:
        row = etf_dict.get(f"{prefix}{code}")
        if row is not None:
            latest_price = float(row.get("最新价", 0) or 0)
            update_time = datetime.now().strftime("%H:%M:%S")
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
            update_time = datetime.now().strftime("%H:%M:%S")
            return estimate, update_time
        except ValueError, AttributeError:
            pass
    update_time = datetime.now().strftime("%H:%M:%S")
    return latest_price, update_time


def get_fund_data_full(code: str, alias: str = "") -> FundData:
    fund = FundData(code=code, alias=alias)
    today = datetime.now()

    try:
        cached_data = _get_cached_fund_data()
        if cached_data is None:
            cached_data = _fetch_and_cache_fund_data()

        if cached_data is not None:
            cached_data_dict = _get_fund_data_dict(cached_data)
            row = cached_data_dict.get(code)
            if row:
                fund.name = str(row.get("基金简称", alias or code))
                nav_cols = sorted(
                    key for key in row if "单位净值" in key and row.get(key)
                )
                if nav_cols:
                    nav_col = nav_cols[-1]
                    fund.nav = float(row.get(nav_col, 0) or 0)
                    if len(nav_cols) >= 2:
                        prev_nav_col = nav_cols[-2]
                        fund.prev_nav = float(row.get(prev_nav_col, 0) or 0)

                    fund.day_change = fund.nav - fund.prev_nav if fund.prev_nav else 0.0

                    acc_nav_col = nav_col.replace("单位", "累计")
                    fund.acc_nav = float(row.get(acc_nav_col, 0) or 0)
                    if "-" in nav_col:
                        parts = nav_col.split("-")
                        fund.nav_date = (
                            "-".join(parts[:3]) if len(parts) >= 3 else parts[0]
                        )
                    else:
                        fund.nav_date = today.strftime("%Y-%m-%d")

                if fund.nav > 0:
                    estimate, update_time = _lookup_etf_estimate(code)
                    if estimate > 0:
                        fund.estimate_nav = estimate
                        fund.estimate_time = update_time
                    else:
                        estimate, update_time = _lookup_cached_estimate(code)
                        if estimate > 0:
                            fund.estimate_nav = estimate
                            fund.estimate_time = update_time
                    return fund
    except Exception:
        pass

    try:
        etf_dict = _get_etf_dict()
        for prefix in ["sz", "sh"]:
            row = etf_dict.get(f"{prefix}{code}")
            if row is not None:
                fund.name = str(row.get("名称", alias or code))
                fund.estimate_nav = float(row.get("最新价", 0) or 0)
                fund.nav = fund.estimate_nav
                prev_close = float(row.get("昨收", 0) or 0)
                if prev_close > 0 and fund.estimate_nav > 0:
                    fund.day_change = fund.estimate_nav - prev_close
                fund.nav_date = today.strftime("%Y-%m-%d")
                fund.estimate_time = datetime.now().strftime("%H:%M:%S")
                return fund
    except Exception:
        pass

    return fund


def get_realtime_estimate(code: str) -> tuple[float, str]:
    estimate, update_time = _lookup_etf_estimate(code)
    if estimate > 0:
        return estimate, update_time
    return _lookup_cached_estimate(code)


def fetch_prev_nav(fund: FundData) -> FundData:
    if fund.prev_nav != 0 or fund.nav == 0:
        return fund
    try:
        import akshare as ak

        hist_df = ak.fund_open_fund_info_em(symbol=fund.code, indicator="单位净值走势")
        if hist_df is not None and len(hist_df) >= 2:
            import copy

            updated = copy.copy(fund)
            updated.prev_nav = float(hist_df.iloc[-2]["单位净值"])
            updated.day_change = updated.nav - updated.prev_nav
            return updated
    except Exception:
        pass
    return fund


def search_fund(keyword: str) -> list[dict]:
    """Search fund

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
            ][:10]  # Limit to 10 results

            for row in matched:
                nav_col = next((key for key in row if "单位净值" in key), None)
                results.append(
                    {
                        "基金代码": row.get("基金代码"),
                        "基金名称": row.get("基金简称"),
                        "最新价": row.get(nav_col, "") if nav_col else "",
                        "涨跌幅": row.get("日增长率", ""),
                    }
                )
            if results:
                return results
    except Exception:
        pass

    # Fallback to ETF data source
    try:
        df = _get_etf_data()
        if df is not None and not df.empty:
            df["基金代码"] = df["代码"].str.extract(r"(\d+)")

            matched = df[
                df["基金代码"].str.contains(keyword, na=False)
                | df["名称"].str.contains(keyword, na=False)
            ]

            for _, row in matched.head(10).iterrows():
                results.append(
                    {
                        "基金代码": row["基金代码"],
                        "基金名称": row["名称"],
                        "最新价": row.get("最新价", ""),
                        "涨跌幅": row.get("涨跌幅", ""),
                    }
                )
    except Exception as e:
        print(f"Error searching fund: {e}")

    return results
