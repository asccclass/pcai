---
name: read_calendars
description: Read Google Calendar events and availability. Use this tool to list events for a specific date range.
command: gog calendar events --all --from {{from}} --to {{to}} --json
options:
  calendar_id:
    - primary
---

# Google Calendar (gog) Skill

## Overview
This skill provides capabilities to read Google Calendar data using the `gog calendar` command-line interface. It focuses on listing calendars, retrieving events, and checking availability.

## Core Concepts

### Calendar IDs
- `primary`: The authenticated user's primary calendar.
- Email address: Specific calendar identifier (e.g., `user@example.com`).

### Timezone Handling
- Events are returned with timezone information.
- Use `--json` output for precise parsing of start/end times and timezones.
- Date/Time inputs support RFC3339, relative terms (`today`, `tomorrow`), or simple dates.

---

## Commands Reference

### 1. List Calendars

List all calendars accessible to the user.

```bash
gog calendar calendars
```

**Useful Flags:**
- `--json`: Output structured JSON (recommended for parsing).
- `--min-access-role string`: Filter by access role (e.g., `owner`, `reader`).
- `--show-deleted`: Include deleted calendars.
- `--show-hidden`: Include hidden calendars.

### 2. List Events

Retrieve events from a specific calendar for a given date range.

**Parameters:**
- `from`: Start date/time (e.g., `2025-01-01` or `2025-01-01T09:00:00Z`).
- `to`: End date/time (e.g., `2025-01-31` or `2025-01-31T17:00:00Z`).

#### Basic Usage
```bash
gog calendar events <calendarId> [flags]
```

#### Time-Based Queries
- **Today**: `gog calendar events primary --today`
- **Tomorrow**: `gog calendar events primary --tomorrow`
- **This Week**: `gog calendar events primary --week`
- **Next N Days**: `gog calendar events primary --days 7`
- **Specific Range**:
  ```bash
  gog calendar events primary --from 2025-01-01T00:00:00Z --to 2025-01-31T23:59:59Z
  ```

#### Search
- **Query**: `gog calendar events primary --query "meeting"`

#### Output Formats
- **JSON (Recommended)**: `gog calendar events primary --json`
  - Provides full event details including IDs, HTML links, attendees, and exact timestamps.
- **Fields Selection**: `gog calendar events primary --json --select "summary,start,end"`

### 3. Check Availability (Free/Busy)

Check free/busy information for a set of calendars.

```bash
gog calendar freebusy --calendars "primary,other@example.com" --from 2025-01-01T09:00:00Z --to 2025-01-01T17:00:00Z
```

---

## Best Practices

1.  **Always use `--json` for programmatic access**: The default text output is designed for human readability and may change. JSON provides a stable contract.
2.  **Explicit Time Ranges**: When possible, specify `--from` and `--to` to limit the data fetched and ensure you get the expected time window.
3.  **Timezones**: Be aware that `gog` handles timezones. When parsing JSON, respect the `timeZone` field in the response if present, or the offsets in `dateTime` fields.
