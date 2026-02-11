#!/usr/bin/env python3
import csv
import json
import os
import re
import sys
from datetime import datetime, timezone
from typing import Any, Dict, Iterable, List, Optional, Tuple


def load_input(arg: str) -> Any:
    if arg and os.path.isfile(arg):
        with open(arg, "r", encoding="utf-8") as f:
            return json.load(f)
    return json.loads(arg)


def unwrap_api_payload(value: Any) -> Any:
    if isinstance(value, dict) and "value" in value and len(value) <= 3:
        return value["value"]
    return value


def parse_time(value: Any) -> Tuple[str, Optional[datetime]]:
    if value in (None, "", "N/A"):
        return "N/A", None
    try:
        num = float(value)
        if num > 1e10:
            num = num / 1000.0
        dt = datetime.fromtimestamp(num, tz=timezone.utc)
        return dt.strftime("%Y-%m-%d %H:%M:%S %Z"), dt
    except Exception:
        pass
    try:
        s = str(value).strip()
        if s.endswith("Z"):
            s = s[:-1] + "+00:00"
        dt = datetime.fromisoformat(s)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        else:
            dt = dt.astimezone(timezone.utc)
        return dt.strftime("%Y-%m-%d %H:%M:%S %Z"), dt
    except Exception:
        return "N/A", None


def seconds_between(start: Optional[datetime], end: Optional[datetime]) -> str:
    if not start or not end:
        return "N/A"
    total = int((end - start).total_seconds())
    if total < 0:
        return "N/A"
    h = total // 3600
    m = (total % 3600) // 60
    s = total % 60
    if h:
        return f"{h}h {m}m {s}s"
    if m:
        return f"{m}m {s}s"
    return f"{s}s"


def format_seconds(value: Any) -> str:
    try:
        total = int(float(value))
    except Exception:
        return "N/A"
    if total < 0:
        return "N/A"
    h = total // 3600
    m = (total % 3600) // 60
    s = total % 60
    if h:
        return f"{h}h {m}m {s}s"
    if m:
        return f"{m}m {s}s"
    return f"{s}s"


def to_mb(size_any: Any) -> int:
    try:
        num = float(size_any)
        if num > 1024 * 1024 * 5:
            return int(round(num / (1024.0 * 1024.0)))
        return int(round(num))
    except Exception:
        return 0


def find_all(obj: Any, keys: Iterable[str]) -> Dict[str, Any]:
    found: Dict[str, Any] = {}

    def walk(node: Any) -> None:
        if isinstance(node, dict):
            for key, value in node.items():
                if key in keys and key not in found and value not in (None, "", []):
                    found[key] = value
                walk(value)
        elif isinstance(node, list):
            for item in node:
                walk(item)

    walk(obj)
    return found


def normalize_schedules(schedules_obj: Any) -> List[Dict[str, Any]]:
    schedules_obj = unwrap_api_payload(schedules_obj)
    if not schedules_obj:
        return []
    if isinstance(schedules_obj, list):
        return [x for x in schedules_obj if isinstance(x, dict)]
    if isinstance(schedules_obj, dict):
        if "schedules" in schedules_obj:
            return normalize_schedules(schedules_obj["schedules"])
        values = []
        for sid, sv in schedules_obj.items():
            if isinstance(sv, dict):
                v = dict(sv)
                v.setdefault("id", sid)
                values.append(v)
        return values
    return []


def get_enabled(schedule: Dict[str, Any]) -> bool:
    for key in ("enabled", "enable"):
        if key in schedule:
            return bool(schedule.get(key))
    for key in ("state", "status"):
        value = schedule.get(key)
        if isinstance(value, bool):
            return value
        if isinstance(value, str) and value.lower() in ("true", "enabled", "enable", "on", "active", "1"):
            return True
    nested = find_all(schedule, {"enabled", "enable", "state", "status"})
    value = nested.get("enabled") or nested.get("enable") or nested.get("state") or nested.get("status")
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        return value.lower() in ("true", "enabled", "enable", "on", "active", "1")
    return False


def get_location(schedule: Dict[str, Any]) -> str:
    for key in ("location", "location_url", "url", "target", "dest", "destination"):
        value = schedule.get(key)
        if value:
            return str(value)
    nested = find_all(schedule, {"location", "location_url", "url", "target", "dest", "destination"})
    for key in ("location", "location_url", "url", "target", "dest", "destination"):
        value = nested.get(key)
        if value:
            return str(value)
    return "N/A"


def guess_location_type(location: Optional[str], schedule: Dict[str, Any]) -> str:
    for key in ("location_type", "type", "protocol", "scheme"):
        value = schedule.get(key)
        if value:
            return str(value).upper()
    nested = find_all(schedule, {"location_type", "type", "protocol", "scheme"})
    for key in ("location_type", "type", "protocol", "scheme"):
        value = nested.get(key)
        if value:
            return str(value).upper()
    if location and "://" in location:
        return location.split("://", 1)[0].upper()
    return "N/A"


def _fmt_time(hour: Any, minute: Any, time_str: Optional[str]) -> Optional[str]:
    if isinstance(time_str, str) and re.match(r"^\d{1,2}:\d{2}$", time_str.strip()):
        hh, mm = time_str.strip().split(":")
        return f"{int(hh):02d}:{int(mm):02d}"
    if isinstance(hour, int) and isinstance(minute, int):
        return f"{hour:02d}:{minute:02d}"
    return None


def extract_recurrence(schedule: Dict[str, Any]) -> str:
    recurrence = schedule.get("recurrence") or schedule.get("schedule") or schedule.get("recurrence_info") or {}
    nested = find_all(
        schedule,
        {"hour", "minute", "days", "period", "type", "time", "cron", "cron_expr", "cron_expression", "frequency", "interval"},
    )
    hour = nested.get("hour", recurrence.get("hour"))
    minute = nested.get("minute", recurrence.get("minute"))
    time_str = nested.get("time")
    cron = nested.get("cron") or nested.get("cron_expr") or nested.get("cron_expression")
    period = nested.get("period") or nested.get("type") or nested.get("frequency") or nested.get("interval")
    days = nested.get("days") if "days" in nested else recurrence.get("days")
    hhmm = _fmt_time(hour, minute, time_str)

    if cron:
        return f"CRON {cron}"

    days_str = None
    if isinstance(days, list) and days:
        day_map = {
            "1": "MONDAY",
            "2": "TUESDAY",
            "3": "WEDNESDAY",
            "4": "THURSDAY",
            "5": "FRIDAY",
            "6": "SATURDAY",
            "7": "SUNDAY",
        }

        def fmt_day(day: Any) -> str:
            ds = str(day).upper()
            return day_map.get(ds, ds[:3] if len(ds) > 3 else ds)

        days_str = ",".join(fmt_day(day) for day in days)

    if days_str and hhmm:
        return f"{(str(period).upper() if period else 'WEEKLY')} {hhmm} {days_str}"
    if period and hhmm:
        return f"{str(period).upper()} {hhmm}"
    if hhmm:
        return f"DAILY {hhmm}"
    return "N/A"


def extract_retention(schedule: Dict[str, Any]) -> str:
    nested = find_all(schedule, {"max_count", "num_backups", "count", "days", "weeks", "months", "retention", "retention_info", "retention_policy"})
    pieces = []
    for key in ("max_count", "num_backups", "count", "days", "weeks", "months"):
        value = nested.get(key)
        if value not in (None, "", []):
            pieces.append(f"{key}={value}")
    if pieces:
        return ", ".join(pieces)
    for key in ("retention", "retention_info", "retention_policy"):
        retention = nested.get(key) or schedule.get(key)
        if isinstance(retention, dict):
            inner = []
            for inner_key in ("max_count", "count", "days", "weeks", "months"):
                inner_val = retention.get(inner_key)
                if inner_val not in (None, "", []):
                    inner.append(f"{inner_key}={inner_val}")
            if inner:
                return ", ".join(inner)
    return "N/A"


def _looks_like_job(node: Dict[str, Any]) -> bool:
    candidate_keys = {"status", "start_time", "end_time", "duration", "operation", "service", "size", "backup_size"}
    return any(key in node for key in candidate_keys)


def _collect_job_candidates(job_obj: Any) -> List[Dict[str, Any]]:
    candidates: List[Dict[str, Any]] = []
    job_obj = unwrap_api_payload(job_obj)
    if isinstance(job_obj, list):
        for item in job_obj:
            if isinstance(item, dict):
                candidates.extend(_collect_job_candidates(item))
        return candidates
    if not isinstance(job_obj, dict):
        return candidates

    if _looks_like_job(job_obj):
        candidates.append(job_obj)

    for key in ("details", "last", "latest", "most_recent", "summary"):
        nested = job_obj.get(key)
        if isinstance(nested, (dict, list)):
            candidates.extend(_collect_job_candidates(nested))

    for job_id, value in job_obj.items():
        if isinstance(value, dict) and _looks_like_job(value):
            candidate = dict(value)
            candidate.setdefault("_job_id", str(job_id))
            candidates.append(candidate)

    return candidates


def _rank_job(job: Dict[str, Any]) -> Tuple[float, float]:
    end_str, end_dt = parse_time(job.get("end_time") or job.get("end") or job.get("completed") or job.get("completion_time") or job.get("finish_time"))
    start_str, start_dt = parse_time(job.get("start_time") or job.get("start") or job.get("started") or job.get("begin_time") or job.get("queued_time"))
    end_score = end_dt.timestamp() if end_dt else -1.0
    start_score = start_dt.timestamp() if start_dt else -1.0
    if end_score < 0:
        job_id = str(job.get("_job_id", ""))
        m = re.match(r"^(\d{8})-(\d{6})", job_id)
        if m:
            id_dt = datetime.strptime(f"{m.group(1)}{m.group(2)}", "%Y%m%d%H%M%S").replace(tzinfo=timezone.utc)
            end_score = id_dt.timestamp()
    return end_score, start_score


def extract_last_job(job_obj: Any) -> Dict[str, Any]:
    candidates = _collect_job_candidates(job_obj)
    if not candidates:
        return {"status": "NO JOB", "start_str": "N/A", "end_str": "N/A", "duration_str": "N/A", "size_mb": 0}

    last_job = sorted(candidates, key=_rank_job, reverse=True)[0]
    status = last_job.get("status") or last_job.get("state") or last_job.get("result") or "UNKNOWN"
    start_any = last_job.get("start_time") or last_job.get("start") or last_job.get("started") or last_job.get("begin_time") or last_job.get("queued_time")
    end_any = last_job.get("end_time") or last_job.get("end") or last_job.get("completed") or last_job.get("completion_time") or last_job.get("finish_time")
    start_str, start_dt = parse_time(start_any)
    end_str, end_dt = parse_time(end_any)
    duration_str = seconds_between(start_dt, end_dt)
    if duration_str == "N/A" and last_job.get("duration") is not None:
        duration_str = format_seconds(last_job.get("duration"))
    size_any = last_job.get("size") or last_job.get("backup_size") or last_job.get("bytes_transferred") or last_job.get("transferred") or last_job.get("total_bytes")
    size_mb = to_mb(size_any)
    return {"status": str(status).upper(), "start_str": start_str, "end_str": end_str, "duration_str": duration_str, "size_mb": size_mb}


def build_rows(all_data: List[Dict[str, Any]]) -> List[List[Any]]:
    rows: List[List[Any]] = []
    for item in all_data:
        item = unwrap_api_payload(item)
        hostname = item.get("hostname", "N/A")
        version = item.get("version") or "N/A"
        build = item.get("build") or "N/A"
        timezone_val = item.get("timezone") or "N/A"
        schedules = normalize_schedules(item.get("schedules"))
        job = extract_last_job(item.get("job_details"))

        if not schedules:
            rows.append(
                [
                    hostname,
                    version,
                    build,
                    timezone_val,
                    0,
                    "N/A",
                    "N/A",
                    "N/A",
                    "N/A",
                    "N/A",
                    "N/A",
                    job["status"],
                    job["start_str"],
                    job["end_str"],
                    job["duration_str"],
                    job["size_mb"],
                ]
            )
            continue

        for schedule in schedules:
            enabled = 1 if get_enabled(schedule) else 0
            location = get_location(schedule)
            loc_type = guess_location_type(location, schedule)
            recurrence = extract_recurrence(schedule)
            retention = extract_retention(schedule)
            rows.append(
                [
                    hostname,
                    version,
                    build,
                    timezone_val,
                    len(schedules),
                    enabled,
                    location,
                    loc_type,
                    recurrence,
                    retention,
                    "Last",
                    job["status"],
                    job["start_str"],
                    job["end_str"],
                    job["duration_str"],
                    job["size_mb"],
                ]
            )
    return rows


def write_csv(all_data: List[Dict[str, Any]], output_path: str) -> None:
    headers = [
        "vCenter",
        "Version",
        "Build",
        "Timezone",
        "Schedules",
        "Enabled",
        "Backup Location",
        "Type",
        "Recurrence",
        "Retention",
        "Last Job",
        "Status",
        "Start",
        "End",
        "Duration",
        "Size (MB)",
    ]
    rows = build_rows(all_data)
    with open(output_path, "w", encoding="utf-8", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(headers)
        writer.writerows(rows)


def main() -> None:
    if len(sys.argv) < 2:
        print("Usage: generate_backup_report_csv.py <json-string-or-file> <output-csv>", file=sys.stderr)
        sys.exit(2)
    input_arg = sys.argv[1]
    output_csv = sys.argv[2] if len(sys.argv) > 2 else "vcenter_backup_report.csv"
    try:
        all_data = load_input(input_arg)
    except Exception as exc:
        print(f"Failed to load input data: {exc}", file=sys.stderr)
        sys.exit(3)
    if not isinstance(all_data, list) or not all_data:
        print("Input JSON is empty or not a list", file=sys.stderr)
        sys.exit(4)
    write_csv(all_data, output_csv)
    print(f"Wrote report to {output_csv}")


if __name__ == "__main__":
    main()
