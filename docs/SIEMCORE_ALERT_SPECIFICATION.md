# SIEMCORE Alert Data Specification

| | |
|---|---|
| **Document Version** | 1.0.0 |
| **Last Updated** | February 5, 2026 |
| **Status** | Draft |
| **Purpose** | Define the alert data structure SIEMCORE must provide to MySoc |

---

## Overview

MySoc AI uses alert data from SIEMCORE to:
1. **Generate accurate client emails** - Without proper context, MySoc hallucinates
2. **Render attack chain visualizations** - Show kill chain progression graphically
3. **Create meaningful tickets** - Like TKT-2026-0147 in the screenshot

**Current Problem**: SIEMCORE alerts lack critical context (source IP, user, file path, parent process, etc.), forcing MySoc to guess or omit important details.

---

## Alert API Endpoint

SIEMCORE must expose an API endpoint that the siemcore-updater can query:

```
GET /api/v1/alerts?since={cursor}&limit={n}
```

**Parameters:**
- `since` - Alert ID or timestamp cursor (for incremental fetch)
- `limit` - Max alerts to return (default: 100)

**Response:**
```json
{
  "alerts": [...],
  "next_cursor": "alert-uuid-or-timestamp",
  "has_more": true
}
```

---

## Required Alert Fields

These fields are **required** for MySoc to function without hallucinating:

### Core Identification

| Field | Type | Required | Example | Why Needed |
|-------|------|----------|---------|------------|
| `alert_id` | string | **YES** | `"alert-550e8400-e29b-41d4"` | Unique identifier, deduplication |
| `alert_name` | string | **YES** | `"Sentinelone: Rivhit_AA_v3.exe on TYA8FApt5j"` | Display in ticket header |
| `severity` | string | **YES** | `"critical"` | Priority routing, email urgency |
| `message` | string | **YES** | `"[MALWARE] EDR Detection: Rivhit_AA_v3.exe"` | Email summary, ticket body |
| `time` | ISO8601 | **YES** | `"2026-02-05T17:38:07Z"` | Timeline, SLA calculation |

### Entity Context (At Least One Required)

MySoc MUST know what entity is affected. Provide at least one:

| Field | Type | Required | Example | Why Needed |
|-------|------|----------|---------|------------|
| `hostname` | string | **YES*** | `"TYA8FApt5j"` | Email: "Alert on server X" |
| `endpoint_id` | string | Recommended | `"endpoint-guid-1234"` | Stable ID for correlation |
| `username` | string | **YES*** | `"john.doe"` | Email: "User X executed malware" |
| `user_id` | string | Recommended | `"azure-ad-object-id"` | Stable ID for correlation |
| `source_ip` | string | **YES*** | `"192.168.1.100"` | Email: "Attack from IP X" |

> *At least ONE of hostname, username, or source_ip must be present

### MITRE ATT&CK Mapping (Required for Attack Chain)

| Field | Type | Required | Example | Why Needed |
|-------|------|----------|---------|------------|
| `tactic_id` | string | **YES** | `"TA0002"` | Kill chain phase |
| `tactic_name` | string | **YES** | `"Execution"` | Human-readable phase |
| `technique_id` | string | **YES** | `"T1059"` | Specific technique |
| `technique_name` | string | **YES** | `"Command and Scripting Interpreter"` | Human-readable technique |

**Reference**: From the screenshot, SIEMCORE already provides T1059 and T1204 - but needs tactic context.

---

## Recommended Alert Fields

These fields significantly improve MySoc's output quality:

### Network Context

| Field | Type | Example | Why Needed |
|-------|------|---------|------------|
| `source_ip` | string | `"192.168.1.100"` | Attack origin |
| `source_port` | int | `49152` | Connection details |
| `destination_ip` | string | `"10.0.0.50"` | Target system |
| `destination_port` | int | `443` | Service targeted |
| `protocol` | string | `"TCP"` | Connection type |

### Process Context (Critical for Malware Alerts)

| Field | Type | Example | Why Needed |
|-------|------|---------|------------|
| `process_name` | string | `"Rivhit_AA_v3.exe"` | What ran |
| `process_path` | string | `"C:\\Users\\john\\Downloads\\Rivhit_AA_v3.exe"` | Where it came from |
| `process_hash` | string | `"sha256:a1b2c3..."` | IOC for threat intel |
| `parent_process` | string | `"outlook.exe"` | How it was launched |
| `command_line` | string | `"Rivhit_AA_v3.exe -silent"` | What it did |

### File Context

| Field | Type | Example | Why Needed |
|-------|------|---------|------------|
| `file_name` | string | `"Rivhit_AA_v3.exe"` | Malware name |
| `file_path` | string | `"C:\\Users\\john\\Downloads\\"` | Location on disk |
| `file_hash` | string | `"sha256:..."` | IOC |
| `file_size` | int | `1048576` | Context |

### Threat Intelligence

| Field | Type | Example | Why Needed |
|-------|------|---------|------------|
| `threat_name` | string | `"Rivhit Ransomware"` | Known threat identification |
| `threat_actor` | string | `"APT29"` | Attribution (if known) |
| `confidence` | int | `90` | How sure (0-100) |
| `verdict` | string | `"True Positive - Confirmed malicious"` | Analyst decision |

### Action Taken

| Field | Type | Example | Why Needed |
|-------|------|---------|------------|
| `action_taken` | string | `"blocked"` | What EDR did |
| `recommended_action` | string | `"Isolate endpoint, investigate"` | Guidance for SOC |
| `status` | string | `"new"` | Ticket workflow |

---

## Attack Chain / Incident Grouping

For MySoc to render attack chain visualizations (like the "Attack Chain" tab in the screenshot), SIEMCORE should group related alerts:

### Per-Alert Fields

| Field | Type | Example | Why Needed |
|-------|------|---------|------------|
| `incident_id` | string | `"INC-2026-0147"` | Groups alerts into attack chain |
| `sequence_number` | int | `1` | Order in chain |

### Incident Object (Optional but Valuable)

If SIEMCORE can provide incident groupings:

```json
{
  "incident_id": "INC-2026-0147",
  "incident_name": "[MALWARE] Rivhit Attack Chain on TYA8FApt5j",
  "severity": "critical",
  "status": "investigating",
  
  "primary_entity_type": "endpoint",
  "primary_entity_id": "endpoint-guid-1234",
  "primary_entity_name": "TYA8FApt5j",
  
  "alert_ids": ["alert-1", "alert-2", "alert-3"],
  "start_alert_id": "alert-1",
  
  "edges": [
    {"from_alert_id": "alert-1", "to_alert_id": "alert-2", "relation": "followed_by"},
    {"from_alert_id": "alert-2", "to_alert_id": "alert-3", "relation": "followed_by"}
  ],
  
  "tactics_used": ["TA0001", "TA0002", "TA0003"],
  "techniques_used": ["T1566", "T1059", "T1547"],
  
  "start_time": "2026-02-05T17:38:07Z"
}
```

---

## Complete Alert Example

Here's a complete alert that would enable MySoc to generate accurate emails and visualizations:

```json
{
  "alert_id": "alert-550e8400-e29b-41d4-a716-446655440000",
  "alert_name": "Sentinelone: Rivhit_AA_v3.exe on TYA8FApt5j",
  "severity": "critical",
  "message": "[MALWARE] EDR Detection: Rivhit_AA_v3.exe executed on endpoint TYA8FApt5j",
  "time": "2026-02-05T17:38:07Z",
  
  "hostname": "TYA8FApt5j",
  "endpoint_id": "endpoint-550e8400-e29b-41d4",
  "endpoint_type": "workstation",
  "endpoint_os": "Windows 11",
  
  "username": "john.doe",
  "user_domain": "CORP",
  "user_id": "user-azure-ad-12345",
  
  "source_ip": "192.168.1.100",
  "destination_ip": "185.234.72.1",
  "destination_port": 443,
  "protocol": "TCP",
  
  "tactic_id": "TA0002",
  "tactic_name": "Execution",
  "technique_id": "T1059",
  "technique_name": "Command and Scripting Interpreter",
  
  "process_name": "Rivhit_AA_v3.exe",
  "process_path": "C:\\Users\\john.doe\\Downloads\\Rivhit_AA_v3.exe",
  "process_hash": "sha256:a1b2c3d4e5f6...",
  "parent_process": "chrome.exe",
  "command_line": "Rivhit_AA_v3.exe",
  
  "threat_name": "Rivhit Ransomware Variant",
  "confidence": 90,
  "verdict": "True Positive - Confirmed malicious activity",
  
  "action_taken": "blocked",
  "recommended_action": "1. Isolate endpoint\n2. Investigate lateral movement\n3. Check for persistence",
  "status": "new",
  
  "incident_id": "INC-2026-0147",
  "sequence_number": 1,
  
  "iocs": [
    "sha256:a1b2c3d4e5f6...",
    "185.234.72.1",
    "rivhit-c2.malware.com"
  ],
  
  "metadata": {
    "edr_vendor": "SentinelOne",
    "rule_name": "Malware Detection - Rivhit Family",
    "detection_source": "static_analysis"
  }
}
```

---

## What MySoc Can Do With This Data

### Email Generation (No Hallucination)

```
Subject: [CRITICAL] Malware Detected on TYA8FApt5j - Immediate Action Required

Alert: Rivhit_AA_v3.exe executed on endpoint TYA8FApt5j

Affected User: john.doe (CORP domain)
Endpoint: TYA8FApt5j (Windows 11 workstation)
Source IP: 192.168.1.100
C2 Connection: 185.234.72.1:443

MITRE ATT&CK: T1059 - Command and Scripting Interpreter (Execution phase)

Threat: Rivhit Ransomware Variant (90% confidence)
Verdict: True Positive - Confirmed malicious activity

Action Taken: Blocked by EDR

Recommended Actions:
1. Isolate endpoint TYA8FApt5j immediately
2. Investigate lateral movement from 192.168.1.100
3. Check for persistence mechanisms

IOCs to block:
- sha256:a1b2c3d4e5f6...
- 185.234.72.1
- rivhit-c2.malware.com
```

### Attack Chain Visualization

```
┌─────────────────────────────────────────────────────────────────┐
│  ATTACK CHAIN: INC-2026-0147                                    │
│  Primary Entity: TYA8FApt5j (workstation)                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  [1] Initial Access    →  [2] Execution      →  [3] C2         │
│      T1566 Phishing        T1059 Scripting       T1071 HTTP    │
│      chrome.exe            Rivhit_AA_v3.exe      185.234.72.1  │
│           │                      │                    │         │
│           └──────────────────────┴────────────────────┘         │
│                         john.doe @ TYA8FApt5j                   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Field Priority Matrix

| Priority | Fields | Impact if Missing |
|----------|--------|-------------------|
| **P0 - Critical** | alert_id, severity, message, time, (hostname OR username OR source_ip) | MySoc cannot function |
| **P1 - High** | tactic_id, tactic_name, technique_id, technique_name | No attack chain visualization |
| **P2 - Medium** | process_path, parent_process, file_hash, destination_ip | Email lacks investigation details |
| **P3 - Nice to Have** | incident_id, sequence_number, edges | Server-side grouping fallback |

---

## Implementation Checklist for SIEMCORE

- [ ] Expose `GET /api/v1/alerts` endpoint with cursor support
- [ ] Include P0 fields in every alert (critical)
- [ ] Include P1 fields for MITRE mapping (high priority)
- [ ] Include P2 fields for context (medium priority)
- [ ] Map EDR vendor alerts to this schema
- [ ] Generate stable `endpoint_id` and `user_id` (not just names)
- [ ] Assign `incident_id` to group related alerts
- [ ] Return `next_cursor` for incremental fetching

---

## Questions for SIEMCORE Team

1. **Where are alerts stored?** (Elasticsearch, PostgreSQL, other?)
2. **What EDR vendors are integrated?** (SentinelOne, CrowdStrike, Defender?)
3. **Does SIEMCORE already correlate alerts into incidents?**
4. **What's the alert volume?** (alerts/day to size payload limits)
5. **Is there an existing alert API we can extend?**
