# Beads - Clean Room Technical Specification

**Version:** 1.0
**Purpose:** This specification describes the requirements and behaviors for implementing a distributed, git-backed issue tracking system designed for AI agents.

---

## 1. Introduction

### 1.1 Overview

Beads is a distributed issue tracking system that stores data in git repositories using a human-readable format. It is designed to serve as persistent, structured memory for AI agents, replacing unstructured markdown with dependency-aware graphs.

### 1.2 Design Goals

1. **Git-Native Persistence**: All data must be stored in formats that work well with git (text-based, diff-friendly, merge-capable)
2. **Distributed Operation**: Multiple clones of a repository must be able to work independently and merge changes
3. **Offline-First**: Full functionality without network connectivity
4. **Agent-Optimized**: Designed for programmatic access by AI agents with limited context windows
5. **Atomic Operations**: All operations must maintain data integrity

### 1.3 Terminology

| Term | Definition |
|------|------------|
| **Issue** | The primary work item entity |
| **Dependency** | A directed relationship between two issues |
| **Tombstone** | A soft-deleted issue that remains in storage for distributed consistency |
| **Molecule** | A reusable template for common work patterns |
| **Beads Directory** | The `.beads/` folder containing all tracker data |

---

## 2. Data Model

### 2.1 Issue Entity

An Issue is the primary entity representing a unit of work. Implementations MUST support all required fields and SHOULD support optional fields.

#### 2.1.1 Required Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `id` | string | Unique, immutable | Unique identifier for the issue |
| `title` | string | Non-empty, max 500 chars | Brief summary of the issue |
| `status` | enum | See §2.1.4 | Current workflow state |
| `created_at` | timestamp | RFC3339 format | When the issue was created |
| `updated_at` | timestamp | RFC3339 format | When the issue was last modified |

#### 2.1.2 Optional Content Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `description` | string | Max 64KB | Detailed description |
| `design` | string | Max 64KB | Technical design notes |
| `acceptance_criteria` | string | Max 16KB | Definition of done |
| `notes` | string | Max 64KB | Freeform notes (append-only merge behavior) |

#### 2.1.3 Optional Metadata Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `priority` | integer | 0-4 inclusive | 0=critical, 4=low; default=2 |
| `issue_type` | string | Suggested: task, bug, feature, epic, story | Classification of work |
| `assignee` | string | Max 256 chars | Current worker identifier |
| `owner` | string | Max 256 chars | Accountable party identifier |
| `estimated_minutes` | integer | Non-negative | Time estimate in minutes |
| `due_at` | timestamp | RFC3339 format | Deadline |
| `defer_until` | timestamp | RFC3339 format | Do not surface until this time |
| `external_ref` | string | Valid URI | Link to external system |
| `source_system` | string | Max 64 chars | Origin system identifier |
| `created_by` | string | Max 256 chars | Creator identifier |

#### 2.1.4 Status Values

Implementations MUST support these status values:

| Status | Description | Transitions To |
|--------|-------------|----------------|
| `open` | Available for work | in-progress, blocked, deferred, closed, tombstone |
| `in-progress` | Currently being worked | open, blocked, closed, tombstone |
| `blocked` | Waiting on dependencies | open, in-progress, closed, tombstone |
| `deferred` | Postponed | open, tombstone |
| `closed` | Completed or resolved | open (reopen), tombstone |
| `tombstone` | Soft-deleted | (terminal state) |

#### 2.1.5 Tombstone Fields

When `status` = `tombstone`, these fields MUST be populated:

| Field | Type | Description |
|-------|------|-------------|
| `deleted_at` | timestamp | When deletion occurred |
| `deleted_by` | string | Who performed deletion |
| `delete_reason` | string | Why the issue was deleted |
| `original_type` | string | Issue type before deletion |

#### 2.1.6 Closure Fields

When `status` = `closed`, these fields SHOULD be populated:

| Field | Type | Description |
|-------|------|-------------|
| `closed_at` | timestamp | When closure occurred |
| `close_reason` | string | Resolution description |
| `closed_by_session` | string | Session/actor that closed |

### 2.2 Dependency Entity

Dependencies represent directed relationships between issues.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `issue_id` | string | Must exist | The dependent issue |
| `depends_on_id` | string | Must exist | The issue being depended upon |
| `type` | enum | See below | Nature of the relationship |
| `created_at` | timestamp | RFC3339 | When created |
| `created_by` | string | Optional | Who created |

#### 2.2.1 Dependency Types

| Type | Semantics |
|------|-----------|
| `blocks` | `depends_on_id` must close before `issue_id` can proceed |
| `related` | Informational link, no workflow implications |
| `parent-child` | Hierarchical containment relationship |
| `discovered-from` | Tracks issue origin/derivation |
| `conditional-blocks` | Blocks only under certain conditions (metadata-dependent) |

### 2.3 Label Entity

Labels are string tags attached to issues.

| Field | Type | Description |
|-------|------|-------------|
| `issue_id` | string | The tagged issue |
| `name` | string | Label text (max 64 chars, case-insensitive for matching) |
| `created_at` | timestamp | When applied |
| `created_by` | string | Who applied |

### 2.4 Comment Entity

Comments are threaded discussions on issues.

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Unique within issue, monotonically increasing |
| `issue_id` | string | The commented issue |
| `author` | string | Comment author |
| `body` | string | Comment text (max 64KB) |
| `created_at` | timestamp | When posted |

### 2.5 Event Entity (Audit Trail)

All state changes MUST be recorded as events.

| Field | Type | Description |
|-------|------|-------------|
| `issue_id` | string | Affected issue |
| `event_type` | string | Type of change (created, updated, status-changed, commented, closed, reopened, deleted) |
| `actor` | string | Who made the change |
| `timestamp` | timestamp | When it occurred |
| `changes` | object | Field-level before/after values |
| `reason` | string | Optional context |

---

## 3. Persistence Format

### 3.1 Directory Structure

All data resides in a `.beads/` directory at the repository root:

```
.beads/
├── issues.jsonl          # Primary issue storage
├── config.yaml           # Repository configuration
├── molecules.jsonl       # Template catalog (optional)
└── hooks/                # Custom hook scripts (optional)
```

### 3.2 JSONL Format

Issues MUST be stored in newline-delimited JSON (JSONL) format in `issues.jsonl`.

#### 3.2.1 Requirements

1. One complete JSON object per line
2. No trailing commas or multi-line objects
3. UTF-8 encoding
4. Lines sorted by `id` field (lexicographic ascending) for deterministic output
5. Empty values MAY be omitted from serialization
6. Timestamps MUST use RFC3339 format with timezone

#### 3.2.2 Example

```jsonl
{"id":"bd-001","title":"Implement login","status":"closed","priority":1,"created_at":"2024-01-15T10:00:00Z","updated_at":"2024-01-16T14:30:00Z","closed_at":"2024-01-16T14:30:00Z"}
{"id":"bd-002","title":"Add dark mode","status":"open","priority":2,"created_at":"2024-01-15T11:00:00Z","updated_at":"2024-01-15T11:00:00Z","dependencies":[{"issue_id":"bd-002","depends_on_id":"bd-001","type":"blocks","created_at":"2024-01-15T11:00:00Z"}]}
```

#### 3.2.3 Embedded Collections

Dependencies, labels, and comments SHOULD be embedded within the issue object:

```json
{
  "id": "bd-003",
  "title": "Example",
  "dependencies": [{"issue_id": "bd-003", "depends_on_id": "bd-001", "type": "blocks"}],
  "labels": ["backend", "urgent"],
  "comments": [{"id": 1, "author": "alice", "body": "Started work", "created_at": "..."}]
}
```

### 3.3 ID Generation

Issue IDs MUST follow this format: `bd-<identifier>`

#### 3.3.1 Requirements

1. Prefix MUST be `bd-`
2. Identifier MUST be 3-10 lowercase alphanumeric characters
3. IDs MUST be unique within a repository
4. IDs MUST be stable (never change after creation)
5. Implementations SHOULD use content-based hashing for distributed uniqueness

#### 3.3.2 Recommended Algorithm

1. Compute SHA-256 of: `title + created_at + created_by + random_nonce`
2. Encode first 5 bytes as base32 (lowercase, no padding)
3. If collision detected, append incrementing suffix: `bd-abc12-1`, `bd-abc12-2`

---

## 4. Storage Interface

Implementations MUST provide a storage abstraction supporting these operations.

### 4.1 Issue Operations

| Operation | Signature | Description |
|-----------|-----------|-------------|
| Create | `create(issue) → id` | Create new issue, return assigned ID |
| Get | `get(id) → issue` | Retrieve issue by ID |
| Update | `update(issue) → void` | Modify existing issue |
| Delete | `delete(id, reason) → void` | Soft-delete (set tombstone status) |
| List | `list(filter) → issues[]` | Query issues with filter criteria |
| Search | `search(query) → issues[]` | Full-text search |

### 4.2 Dependency Operations

| Operation | Signature | Description |
|-----------|-----------|-------------|
| Add | `add_dependency(from, to, type) → void` | Create dependency |
| Remove | `remove_dependency(from, to) → void` | Delete dependency |
| Get Dependencies | `get_dependencies(id) → deps[]` | What does this issue depend on? |
| Get Dependents | `get_dependents(id) → deps[]` | What depends on this issue? |
| Detect Cycles | `detect_cycles() → cycles[]` | Find circular dependencies |

### 4.3 Query Operations

| Operation | Signature | Description |
|-----------|-----------|-------------|
| Get Ready Work | `get_ready_work() → issues[]` | Open issues with no open blocking dependencies |
| Get By Status | `get_by_status(status) → issues[]` | Filter by status |
| Get By Label | `get_by_label(label) → issues[]` | Filter by label |
| Get By Assignee | `get_by_assignee(assignee) → issues[]` | Filter by assignee |

### 4.4 Transaction Support

Implementations SHOULD support atomic transactions:

| Operation | Description |
|-----------|-------------|
| Begin | Start a transaction |
| Commit | Persist all changes |
| Rollback | Discard all changes |

---

## 5. Synchronization

### 5.1 Sync Modes

Implementations MUST support at least `git-portable` mode:

| Mode | Behavior |
|------|----------|
| `git-portable` | Export to JSONL before push; import from JSONL after pull |
| `realtime` | Export to JSONL on every change |

### 5.2 Export Process

1. Read all issues from storage
2. Sort by ID (ascending lexicographic)
3. Serialize each issue as single-line JSON
4. Write to `.beads/issues.jsonl`
5. Exclude fields with empty/default values

### 5.3 Import Process

1. Parse `.beads/issues.jsonl` line by line
2. For each issue:
   - If ID doesn't exist locally: create
   - If ID exists and content differs: merge (see §5.4)
   - If ID exists and content matches: skip
3. Handle issues that exist locally but not in JSONL (may have been deleted remotely)

### 5.4 Three-Way Merge Algorithm

When the same issue has been modified in multiple clones, use three-way merge:

**Inputs:**
- Base: Common ancestor version
- Left: Local version
- Right: Remote version

**Output:** Merged version

#### 5.4.1 Field-Level Merge Rules

| Field | Merge Strategy |
|-------|----------------|
| `title` | Side with latest `updated_at` wins |
| `description` | Side with latest `updated_at` wins |
| `notes` | Concatenate both changes with separator `\n\n---\n\n` |
| `status` | See §5.4.2 |
| `priority` | Lower number (higher priority) wins |
| `dependencies` | Union of additions; removals are authoritative |

#### 5.4.2 Status Merge Priority

When merging status, apply this precedence (highest first):

1. **tombstone** - Deletion is explicit and permanent
2. **closed** - Closure should not be undone by concurrent edits
3. Standard three-way merge for other statuses

#### 5.4.3 Tombstone Semantics

Tombstones (soft-deleted issues) require special handling:

1. Tombstone status wins over all other statuses
2. Tombstones have a TTL (default: 7 days)
3. After TTL expiration, a live issue MAY "resurrect" a tombstone
4. Clock skew grace period: 1 hour

```
TTL Check:
  if (now - deleted_at) > (TTL + grace_period):
    resurrection_allowed = true
```

#### 5.4.4 Conflict Resolution Strategies

Implementations SHOULD support configurable strategies:

| Strategy | Behavior |
|----------|----------|
| `newest` | Side with latest `updated_at` wins all fields |
| `ours` | Local version wins all conflicts |
| `theirs` | Remote version wins all conflicts |
| `manual` | Prompt user for each conflict |

---

## 6. Configuration

### 6.1 Configuration Hierarchy

Settings are loaded in this order (later overrides earlier):

1. Built-in defaults
2. User config: `~/.config/beads/config.yaml` or `~/.beads/config.yaml`
3. Project config: `.beads/config.yaml`
4. Environment variables: `BEADS_*` prefix

### 6.2 Configuration Schema

```yaml
# Synchronization settings
sync:
  mode: git-portable          # git-portable | realtime
  conflict_strategy: newest   # newest | ours | theirs | manual
  auto_sync: true             # Automatically sync on git operations

# Export settings
export:
  include_closed: true        # Include closed issues in export
  include_tombstones: true    # Include tombstones (required for distributed delete)

# Compaction settings
compaction:
  enabled: false              # Enable automatic compaction
  age_threshold_days: 30      # Compact issues closed longer than this

# Identity
identity:
  name: ""                    # Default author name
  email: ""                   # Default author email
```

### 6.3 Environment Variables

| Variable | Overrides |
|----------|-----------|
| `BEADS_SYNC_MODE` | `sync.mode` |
| `BEADS_CONFLICT_STRATEGY` | `sync.conflict_strategy` |
| `BEADS_DEBUG` | Enable debug logging |

---

## 7. Command-Line Interface

### 7.1 Core Commands

| Command | Arguments | Description |
|---------|-----------|-------------|
| `init` | `[path]` | Initialize beads in a repository |
| `create` | `<title> [--desc] [--type] [--priority]` | Create new issue |
| `list` | `[--status] [--assignee] [--label]` | List issues |
| `show` | `<id>` | Display issue details |
| `update` | `<id> [--title] [--desc] [--status] ...` | Modify issue |
| `close` | `<id> [--reason]` | Close an issue |
| `reopen` | `<id>` | Reopen a closed issue |
| `delete` | `<id> [--reason]` | Soft-delete an issue |

### 7.2 Dependency Commands

| Command | Arguments | Description |
|---------|-----------|-------------|
| `dep add` | `<from-id> <to-id> [--type]` | Add dependency |
| `dep remove` | `<from-id> <to-id>` | Remove dependency |
| `dep tree` | `<id>` | Show dependency tree |
| `dep cycles` | | Detect circular dependencies |

### 7.3 Organization Commands

| Command | Arguments | Description |
|---------|-----------|-------------|
| `ready` | `[--assignee]` | List ready (unblocked) work |
| `label` | `<id> <add|remove> <label>` | Manage labels |
| `comment` | `<id> <text>` | Add comment |
| `search` | `<query>` | Full-text search |

### 7.4 Sync Commands

| Command | Arguments | Description |
|---------|-----------|-------------|
| `sync` | | Sync with git (export/import as needed) |
| `export` | `[--output]` | Export to JSONL |
| `import` | `[--input]` | Import from JSONL |

### 7.5 Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |
| 3 | Issue not found |
| 4 | Conflict requiring resolution |

---

## 8. Background Daemon (Optional)

Implementations MAY provide a background daemon for improved performance.

### 8.1 Purpose

- Reduce startup latency for CLI commands
- Enable file watching for auto-sync
- Provide async operations without blocking CLI

### 8.2 Communication

- Unix domain socket (POSIX) or named pipe (Windows)
- JSON-based request/response protocol
- Socket path: platform temp directory + unique identifier

### 8.3 Operations

The daemon SHOULD support these RPC operations:

| Operation | Description |
|-----------|-------------|
| `ping` | Health check |
| `create_issue` | Create issue |
| `get_issue` | Retrieve issue |
| `update_issue` | Modify issue |
| `list_issues` | Query issues |
| `sync` | Trigger sync |
| `shutdown` | Graceful shutdown |

### 8.4 Lifecycle

1. CLI checks for running daemon via socket
2. If not running and `auto_daemon: true`, CLI starts daemon
3. Daemon exits after idle timeout (default: 30 minutes)
4. Daemon writes PID file for process management

---

## 9. Hooks

### 9.1 Git Hooks

Implementations SHOULD integrate with these git hooks:

| Hook | Action |
|------|--------|
| `post-commit` | Export JSONL |
| `post-merge` | Import JSONL |
| `post-checkout` | Import JSONL (if branch changed) |
| `pre-push` | Export JSONL, commit if changed |

### 9.2 Custom Hooks

Scripts in `.beads/hooks/` are executed at defined points:

| Hook | Arguments | When |
|------|-----------|------|
| `post-create` | `<issue-id>` | After issue creation |
| `post-update` | `<issue-id>` | After issue modification |
| `post-close` | `<issue-id>` | After issue closure |
| `pre-sync` | | Before sync operation |
| `post-sync` | | After sync operation |

---

## 10. Templates (Molecules)

### 10.1 Purpose

Molecules are reusable templates for common work patterns.

### 10.2 Storage

Templates stored in `.beads/molecules.jsonl` with `is_template: true` field.

### 10.3 Template Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Template identifier (prefix: `mol-`) |
| `title` | string | Template name |
| `is_template` | boolean | Must be `true` |
| `steps` | array | Sequence of issue templates |
| `variables` | object | Variable definitions with defaults |

### 10.4 Variable Substitution

Templates support variable substitution using `{{variable_name}}` syntax:

```yaml
title: "Review PR for {{feature_name}}"
assignee: "{{reviewer}}"
```

### 10.5 Instantiation

When a template is instantiated:
1. Deep copy the template
2. Generate new unique IDs for all issues
3. Substitute all variables with provided values
4. Remove `is_template` flag
5. Create issues and dependencies

---

## 11. Compaction

### 11.1 Purpose

Reduce storage size and AI context window usage by summarizing old completed work.

### 11.2 Eligibility

An issue is eligible for compaction when:
- Status is `closed`
- `closed_at` is older than threshold (default: 30 days)
- No open issues depend on it

### 11.3 Process

1. Generate summary from title, description, and close_reason
2. Set `compaction_level` field (1 = summarized, 2 = archived)
3. Truncate description/notes to summary
4. Preserve: id, title (may shorten), status, key metadata
5. Remove: full description, comments, design docs

### 11.4 Compacted Issue Fields

| Field | Behavior |
|-------|----------|
| `compaction_level` | Set to compaction level |
| `compacted_at` | Timestamp of compaction |
| `original_size` | Byte count before compaction |
| `summary` | AI-generated or extracted summary |

---

## 12. Multi-Repository Federation (Optional)

### 12.1 Routing

Issues can be routed to different repositories based on ID prefix:

```yaml
routing:
  rules:
    - prefix: "frontend-"
      repository: "org/frontend-repo"
    - prefix: "backend-"
      repository: "org/backend-repo"
    - prefix: "*"
      repository: "org/main-repo"
```

### 12.2 Cross-Repository Dependencies

Dependencies MAY reference issues in other repositories using full identifiers:
- Format: `<repository>:<issue-id>`
- Example: `org/frontend-repo:bd-abc12`

### 12.3 Sovereignty Tiers

| Tier | Permissions |
|------|-------------|
| T1 (Full) | Complete read/write control |
| T2 (Shared) | Read/write with merge required |
| T3 (Delegated) | Write requires approval |
| T4 (Read-only) | Read access only |

---

## 13. Invariants and Constraints

### 13.1 Data Integrity

1. Every issue MUST have a unique, non-empty `id`
2. Every issue MUST have a non-empty `title`
3. `updated_at` MUST be >= `created_at`
4. `closed_at` MUST only be set when `status` = `closed`
5. Tombstone fields MUST only be set when `status` = `tombstone`
6. Dependencies MUST NOT create cycles (enforced at creation time)
7. Dependencies MUST reference existing issues

### 13.2 Behavioral Guarantees

1. Deleting an issue MUST NOT orphan dependencies (cascade to tombstone or reject)
2. Closing an issue MUST NOT automatically close dependents
3. Export MUST be deterministic (same data = same output)
4. Import MUST be idempotent (repeated import = no change)

### 13.3 Ordering Guarantees

1. Events for an issue MUST be ordered by timestamp
2. Comments MUST be ordered by ID (monotonically increasing)
3. JSONL export MUST be sorted by issue ID

---

## 14. Error Handling

### 14.1 Error Categories

| Category | Recovery |
|----------|----------|
| Not Found | Return appropriate error, do not create |
| Conflict | Apply conflict resolution strategy |
| Validation | Reject with specific field errors |
| Storage | Retry with backoff, then fail |
| Network | Retry with backoff, then operate offline |

### 14.2 Partial Failure

During bulk operations:
- Continue processing remaining items
- Collect all errors
- Report summary at end
- Rollback if transactional

---

## Appendix A: JSONL Schema (JSON Schema)

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["id", "title", "status", "created_at", "updated_at"],
  "properties": {
    "id": {
      "type": "string",
      "pattern": "^bd-[a-z0-9]{3,10}(-[0-9]+)?$"
    },
    "title": {
      "type": "string",
      "minLength": 1,
      "maxLength": 500
    },
    "status": {
      "type": "string",
      "enum": ["open", "in-progress", "blocked", "deferred", "closed", "tombstone"]
    },
    "priority": {
      "type": "integer",
      "minimum": 0,
      "maximum": 4,
      "default": 2
    },
    "created_at": {
      "type": "string",
      "format": "date-time"
    },
    "updated_at": {
      "type": "string",
      "format": "date-time"
    },
    "dependencies": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["issue_id", "depends_on_id", "type"],
        "properties": {
          "issue_id": {"type": "string"},
          "depends_on_id": {"type": "string"},
          "type": {
            "type": "string",
            "enum": ["blocks", "related", "parent-child", "discovered-from", "conditional-blocks"]
          }
        }
      }
    },
    "labels": {
      "type": "array",
      "items": {"type": "string", "maxLength": 64}
    },
    "comments": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["id", "author", "body", "created_at"],
        "properties": {
          "id": {"type": "integer"},
          "author": {"type": "string"},
          "body": {"type": "string"},
          "created_at": {"type": "string", "format": "date-time"}
        }
      }
    }
  }
}
```

---

## Appendix B: Example Session

```bash
# Initialize in a git repository
$ bd init
Initialized beads in .beads/

# Create issues
$ bd create "Implement user authentication" --type feature --priority 1
Created bd-a1b2c

$ bd create "Write login form" --type task
Created bd-d3e4f

# Add dependency
$ bd dep add bd-d3e4f bd-a1b2c --type blocks
Added: bd-d3e4f blocks bd-a1b2c

# List ready work
$ bd ready
bd-d3e4f  Write login form  [open]  P2

# Update and close
$ bd update bd-d3e4f --status in-progress --assignee alice
Updated bd-d3e4f

$ bd close bd-d3e4f --reason "Completed and tested"
Closed bd-d3e4f

# Sync with git
$ bd sync
Exported 2 issues to .beads/issues.jsonl

$ git add .beads/ && git commit -m "Update issues"
$ git push
```

---

## Appendix C: Glossary

| Term | Definition |
|------|------------|
| **Base** | Common ancestor in three-way merge |
| **Blocking** | A dependency that prevents work from starting |
| **Clean Room** | Implementation without reference to existing code |
| **Deterministic** | Same input always produces same output |
| **Idempotent** | Operation can be repeated without changing result |
| **JSONL** | JSON Lines - newline-delimited JSON format |
| **Ready Work** | Issues with status=open and no open blocking dependencies |
| **Resurrection** | Recreating a tombstoned issue |
| **Tombstone** | Soft-delete marker for distributed consistency |
| **TTL** | Time To Live - expiration duration |
